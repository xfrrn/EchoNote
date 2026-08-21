#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
artifact_dir="$repo_root/output/playwright/browser-smoke"
runtime_dir="$artifact_dir/.playwright-cli/runtime"
web_port="${BROWSER_SMOKE_WEB_PORT:-4173}"
api_port="${BROWSER_SMOKE_API_PORT:-18082}"
for port in "$web_port" "$api_port"; do
  [[ "$port" =~ ^[0-9]+$ ]] && ((10#$port >= 1024 && 10#$port <= 65535)) || { echo "invalid browser smoke port: $port" >&2; exit 1; }
done
((10#$web_port != 10#$api_port)) || { echo "browser smoke ports must differ" >&2; exit 1; }
browser_url="http://127.0.0.1:$web_port"
api_url="http://127.0.0.1:$api_port"
session="echonote-browser-smoke"
playwright_cli_version="0.1.18"
api_pid=""
web_pid=""

: "${DATABASE_URL:?DATABASE_URL must point to a disposable EchoNote test database}"
command -v npx >/dev/null 2>&1 || { echo "npx is required" >&2; exit 1; }

database_name="$(node -e "process.stdout.write(new URL(process.env.DATABASE_URL).pathname.replace(/^\\//, ''))")"
case "$database_name" in
  echonote_test|echonote_test_*|echonote_browser_*) ;;
  *) echo "browser smoke refuses non-test database: $database_name" >&2; exit 1 ;;
esac

mkdir -p "$runtime_dir"

pwcli() {
  npx --yes --package "@playwright/cli@$playwright_cli_version" playwright-cli "-s=$session" "$@"
}

cleanup() {
  local status=$?
  trap - EXIT INT TERM
  set +e
  (cd "$artifact_dir" && pwcli close >/dev/null 2>&1)
  for child_pid in "$web_pid" "$api_pid"; do
    if [[ "$child_pid" =~ ^[1-9][0-9]*$ ]] && kill -0 "$child_pid" 2>/dev/null; then
      kill "$child_pid"
      wait "$child_pid" 2>/dev/null
    fi
  done
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

wait_for() {
  local url="$1"
  local process_id="$2"
  local log_file="$3"
  for _ in {1..80}; do
    if curl --fail --silent --max-time 2 "$url" >/dev/null 2>&1; then
      return
    fi
    if ! kill -0 "$process_id" 2>/dev/null; then
      tail -n 80 "$log_file" >&2
      return 1
    fi
    sleep 0.25
  done
  tail -n 80 "$log_file" >&2
  echo "timed out waiting for $url" >&2
  return 1
}

export APP_ENV=test
export PUBLIC_ORIGIN="$browser_url"
export SERVER_HOST=127.0.0.1
export SERVER_PORT="$api_port"
export PASSWORD_BCRYPT_COST=10
export TRANSCRIPTION_ENABLED=false
unset ECHONOTE_USER_ID ASR_PROVIDER ASR_API_KEY STORAGE_PROVIDER STORAGE_REGION
unset STORAGE_ENDPOINT STORAGE_BUCKET STORAGE_ACCESS_KEY STORAGE_SECRET_KEY
unset EMBEDDING_PROVIDER EMBEDDING_API_KEY LLM_PROVIDER LLM_API_KEY

for occupied_url in "$api_url/healthz" "$browser_url/login"; do
  if curl --silent --show-error --max-time 1 "$occupied_url" >/dev/null 2>&1; then
    echo "browser smoke port is already serving: $occupied_url" >&2
    exit 1
  fi
done

for command in api migrate admin browserfixture; do
  go -C "$repo_root/apps/server" build -trimpath -o "$runtime_dir/echonote-$command" "./cmd/$command"
done
"$runtime_dir/echonote-migrate" up >"$runtime_dir/migrate.log" 2>&1

username="browser_smoke_$(date +%s)_$$"
printf '%s\n' 'BrowserSmoke-2026!' | "$runtime_dir/echonote-admin" create "$username" >"$runtime_dir/admin.log" 2>&1
fixture_json="$("$runtime_dir/echonote-browserfixture" seed "$username")"
episode_id="$(node -e "process.stdout.write(JSON.parse(process.argv[1]).episode_id)" "$fixture_json")"
run_id="$(node -e "process.stdout.write(JSON.parse(process.argv[1]).run_id)" "$fixture_json")"
first_segment_id="$(node -e "process.stdout.write(JSON.parse(process.argv[1]).segment_ids[0])" "$fixture_json")"
for fixture_id in "$episode_id" "$run_id" "$first_segment_id"; do
  [[ "$fixture_id" =~ ^[0-9a-f]{8}-([0-9a-f]{4}-){3}[0-9a-f]{12}$ ]] || {
    echo "browser fixture returned an invalid UUID" >&2
    exit 1
  }
done

"$runtime_dir/echonote-api" >"$runtime_dir/api.log" 2>&1 &
api_pid=$!
ECHONOTE_API_PROXY="$api_url" node "$repo_root/apps/web/node_modules/vite/bin/vite.js" preview \
  "$repo_root/apps/web" --host 127.0.0.1 --port "$web_port" --strictPort >"$runtime_dir/web.log" 2>&1 &
web_pid=$!

wait_for "$api_url/readyz" "$api_pid" "$runtime_dir/api.log"
wait_for "$browser_url/login" "$web_pid" "$runtime_dir/web.log"

cd "$artifact_dir"
pwcli close >/dev/null 2>&1 || true
pwcli open "$browser_url/login" --browser chrome >/dev/null
pwcli snapshot >/dev/null

read -r -d '' smoke_code <<PLAYWRIGHT || true
async page => {
  const origin = await page.evaluate(() => location.origin);
  const assert = (condition, message) => { if (!condition) throw new Error(message); };
  const fixtureEpisodeId = '$episode_id';
  const runId = '$run_id';
  const firstSegmentId = '$first_segment_id';
  const firstSegmentText = 'browserproof transcript evidence from Speaker A';
  const secondSegmentText = 'second browser transcript segment from Speaker B';
  const pageTwoSegmentText = 'browserproof transcript page two evidence';
  const importResponses = new Map();
  const importPosts = [];
  let conversation = null;
  let conversationMessages = [];

  await page.route('**/api/v1/imports**', async route => {
    const request = route.request();
    const path = request.url().replace(/^https?:\/\/[^/]+/, '').split('?')[0];
    if (request.method() === 'POST' && path === '/api/v1/imports') {
      const response = await route.fetch();
      const text = await response.text();
      if (response.ok()) {
        const body = JSON.parse(text);
        importResponses.set(body.id, body);
        importPosts.push(body.url);
      }
      await route.fulfill({ response, body: text });
      return;
    }
    const match = path.match(/^\/api\/v1\/imports\/([^/]+)$/);
    const saved = match && importResponses.get(decodeURIComponent(match[1]));
    if (request.method() === 'GET' && saved) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ ...saved, status: 'succeeded', stage: 'completed', episode_id: fixtureEpisodeId })
      });
      return;
    }
    await route.continue();
  });

  await page.route('**/api/v1/conversations**', async route => {
    const request = route.request();
    const path = request.url().replace(/^https?:\/\/[^/]+/, '').split('?')[0];
    if (request.method() === 'POST' && path === '/api/v1/conversations') {
      const response = await route.fetch();
      const text = await response.text();
      if (response.ok()) conversation = JSON.parse(text);
      await route.fulfill({ response, body: text });
      return;
    }
    if (conversation && request.method() === 'GET' && path === '/api/v1/conversations/' + conversation.id) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ ...conversation, messages: conversationMessages })
      });
      return;
    }
    if (conversation && request.method() === 'POST' && path === '/api/v1/conversations/' + conversation.id + '/messages') {
      const input = request.postDataJSON();
      const timestamp = new Date().toISOString();
      const citation = {
        source_type: 'transcript', source_id: firstSegmentId, excerpt: firstSegmentText,
        speaker_name: 'Speaker A', start_ms: 1000, end_ms: 5000
      };
      conversationMessages = [
        {
          id: '00000000-0000-4000-8000-000000000001', client_message_id: input.client_message_id,
          role: 'user', status: 'completed', content: input.content, citations: [],
          input_tokens: 0, output_tokens: 0, created_at: timestamp, updated_at: timestamp, completed_at: timestamp
        },
        {
          id: '00000000-0000-4000-8000-000000000002', reply_to_message_id: '00000000-0000-4000-8000-000000000001',
          role: 'assistant', status: 'completed', content: 'Browser streamed answer', citations: [citation],
          input_tokens: 3, output_tokens: 4, created_at: timestamp, updated_at: timestamp, completed_at: timestamp
        }
      ];
      const stream =
        'event: delta\ndata: ' + JSON.stringify({ text: 'Browser streamed answer' }) + '\n\n' +
        'event: citation\ndata: ' + JSON.stringify(citation) + '\n\n' +
        'event: done\ndata: ' + JSON.stringify({ message_id: conversationMessages[1].id, replayed: false }) + '\n\n';
      await route.fulfill({
        status: 200,
        headers: { 'content-type': 'text/event-stream; charset=utf-8', 'cache-control': 'no-cache' },
        body: stream
      });
      return;
    }
    await route.continue();
  });

  await page.evaluate(() => new Promise((resolve, reject) => {
    localStorage.removeItem('echonote.capture');
    for (let index = localStorage.length - 1; index >= 0; index -= 1) {
      const key = localStorage.key(index);
      if (key && key.startsWith('echonote.conversation.')) localStorage.removeItem(key);
    }
    sessionStorage.clear();
    const open = indexedDB.open('echonote', 1);
    open.onupgradeneeded = () => open.result.createObjectStore('capture-outbox', { keyPath: 'client_note_id' });
    open.onerror = () => reject(open.error);
    open.onsuccess = () => {
      const transaction = open.result.transaction('capture-outbox', 'readwrite');
      transaction.objectStore('capture-outbox').clear();
      transaction.oncomplete = () => resolve(true);
      transaction.onerror = () => reject(transaction.error);
    };
  }));
  await page.getByRole('textbox', { name: '用户名' }).fill('$username');
  await page.getByRole('textbox', { name: '密码' }).fill('BrowserSmoke-2026!');
  await page.getByRole('button', { name: '登录' }).click();
  await page.waitForFunction(() => location.pathname !== '/login');
  await page.evaluate(() => navigator.serviceWorker.ready.then(() => true));
  await page.reload();
  await page.waitForFunction(() => Boolean(navigator.serviceWorker.controller));

  let episodeId = fixtureEpisodeId;
  const offlineNote = 'Browser CI offline note';
  try {
    await page.goto(origin + '/episode/' + episodeId);
    await page.getByText('Browser CI full-flow episode', { exact: true }).waitFor();
    await page.getByText('browserproof indexed note', { exact: true }).waitFor();

    await page.getByRole('tab', { name: 'Transcript', exact: true }).click();
    await page.getByText('自动转录 · 已区分 2 位说话人', { exact: true }).waitFor();
    await page.getByText(firstSegmentText, { exact: true }).waitFor();
    await page.getByText(secondSegmentText, { exact: true }).waitFor();
    const transcriptionSSE = await page.evaluate(async id => {
      const read = async lastEventId => {
        const response = await fetch('/api/v1/transcriptions/' + encodeURIComponent(id) + '/events', {
          headers: lastEventId === undefined ? {} : { 'Last-Event-ID': String(lastEventId) }
        });
        if (!response.ok) throw new Error(await response.text());
        const text = await response.text();
        return text.split('\n\n').map(block => ({
          id: Number((block.match(/^id:\s*(\d+)$/m) || [])[1]),
          event: (block.match(/^event:\s*(.+)$/m) || [])[1]
        })).filter(item => item.event);
      };
      const all = await read();
      const resumed = await read(all[0].id);
      return { all: all.map(item => item.event), resumed: resumed.map(item => item.event) };
    }, runId);
    assert(transcriptionSSE.all.join(',') === 'started,completed', 'transcription SSE did not replay durable events');
    assert(transcriptionSSE.resumed.join(',') === 'completed', 'transcription SSE resume boundary is wrong');

    await page.getByRole('tab', { name: 'AI', exact: true }).click();
    await page.getByText('browserproof AI summary', { exact: true }).waitFor();
    await page.getByRole('button', { name: '问这期节目…' }).click();
    const chat = page.getByRole('dialog', { name: '问这期节目' });
    await chat.getByRole('textbox', { name: '问这期节目…' }).fill('What is the browser proof?');
    await chat.getByRole('button', { name: '发送' }).click();
    await chat.getByText('Browser streamed answer', { exact: true }).last().waitFor();
    await chat.getByText(/引用：.*browserproof transcript evidence from Speaker A/).last().waitFor();
    assert(conversation && conversation.id, 'conversation was not created by the real API');
    assert(conversationMessages.length === 2, 'AI stream was not persisted into the browser conversation view');
    await chat.getByRole('button', { name: '关闭' }).click();

    await page.context().grantPermissions(['clipboard-read', 'clipboard-write'], { origin });
    await page.getByRole('button', { name: '导出或分享' }).click();
    const share = page.getByRole('dialog', { name: '导出与分享' });
    const copyMode = async (name, segment) => {
      await share.getByRole('radio', { name: new RegExp(name) }).check();
      if (segment) {
        await share.getByRole('button', { name: '载入更多' }).click();
        await share.getByRole('checkbox', { name: new RegExp(segment) }).check();
      }
      await share.getByRole('button', { name: /复制全文|复制到剪贴板/ }).click();
      await share.getByRole('status').filter({ hasText: '已复制全文' }).waitFor();
      return page.evaluate(() => navigator.clipboard.readText());
    };
    const notesExport = await copyMode('仅我的笔记');
    assert(notesExport.includes('browserproof indexed note'), 'notes-only export is missing the fixture note');
    const organizedExport = await copyMode('整理笔记');
    assert(organizedExport.includes('browserproof indexed note') && organizedExport.includes('browserproof AI summary'), 'organized export is incomplete');
    const selectedExport = await copyMode('Transcript 选段', pageTwoSegmentText);
    assert(selectedExport.includes(pageTwoSegmentText) && !selectedExport.includes(firstSegmentText) && !selectedExport.includes(secondSegmentText), 'selected transcript export used the wrong segments');
    const fullExport = await copyMode('完整 Transcript');
    assert(fullExport.includes(firstSegmentText) && fullExport.includes(secondSegmentText) && fullExport.includes(pageTwoSegmentText), 'full transcript export is incomplete');
    await share.getByRole('button', { name: '关闭' }).click();

    await page.goto(origin + '/search');
    await page.getByRole('searchbox').fill('browserproof');
    for (const label of ['我的笔记', 'Transcript', 'AI 整理']) {
      await page.locator('section[aria-label="' + label + '"]').waitFor();
    }
    const searchTypes = await page.evaluate(async () => {
      const response = await fetch('/api/v1/search?q=browserproof&scope=library&limit=50');
      if (!response.ok) throw new Error(await response.text());
      return [...new Set((await response.json()).items.map(item => item.document_type))].sort();
    });
    assert(searchTypes.join(',') === 'ai_artifact,note,transcript', 'keyword search did not cover notes, transcript, and AI');

    await page.goto(origin + '/episode/' + episodeId);
    await page.getByRole('tab', { name: 'Transcript', exact: true }).click();
    await page.getByText('自动转录 · 已区分 2 位说话人', { exact: true }).waitFor();
    assert(await page.getByRole('button', { name: '重命名' }).count() === 2, 'speaker rename controls are missing');
    assert(await page.getByRole('button', { name: '合并' }).count() === 2, 'speaker merge controls are missing');
    const speakerStatuses = await page.evaluate(async id => {
      const transcriptResponse = await fetch('/api/v1/episodes/' + encodeURIComponent(id) + '/transcript');
      const transcript = await transcriptResponse.json();
      if (!transcriptResponse.ok || transcript.speakers.length !== 2) throw new Error(JSON.stringify(transcript));
      const target = transcript.speakers[0];
      const source = transcript.speakers[1];
      const renamed = await fetch('/api/v1/transcripts/' + encodeURIComponent(transcript.id) + '/speakers/' + encodeURIComponent(target.id), {
        method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ display_name: '访谈主持' })
      });
      const merged = await fetch('/api/v1/transcripts/' + encodeURIComponent(transcript.id) + '/speakers/merge', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ source_speaker_id: source.id, target_speaker_id: target.id })
      });
      return [renamed.status, merged.status];
    }, episodeId);
    assert(speakerStatuses.join(',') === '200,200', 'speaker rename or merge failed');
    await page.reload();
    await page.getByRole('tab', { name: 'Transcript', exact: true }).click();
    await page.getByText('自动转录 · 已区分 1 位说话人', { exact: true }).waitFor();
    await page.getByText('访谈主持', { exact: true }).first().waitFor();

    await page.getByRole('tab', { name: '笔记', exact: true }).click();
    assert(await page.evaluate(() => Boolean(navigator.serviceWorker.controller)), 'PWA is not controlled by its service worker');
    await page.context().setOffline(true);
    assert(!(await page.evaluate(() => navigator.onLine)), 'browser did not enter offline mode');
    await page.getByRole('button', { name: '记录想法' }).click();
    await page.getByRole('textbox', { name: '记录想法' }).fill(offlineNote);
    await page.getByRole('button', { name: '完成' }).click();
    await page.waitForURL('**/episode/' + episodeId);
    await page.getByText(offlineNote, { exact: true }).waitFor();
    await page.getByText('离线保存，联网后自动发送', { exact: true }).waitFor();
    try {
      await page.reload({ waitUntil: 'domcontentloaded' });
    } catch (reason) {
      throw new Error('offline PWA reload failed: ' + (reason instanceof Error ? reason.message : String(reason)));
    }
    await page.getByText(offlineNote, { exact: true }).waitFor();
    const readOutbox = () => page.evaluate(() => new Promise((resolve, reject) => {
      const open = indexedDB.open('echonote', 1);
      open.onerror = () => reject(open.error);
      open.onsuccess = () => {
        const request = open.result.transaction('capture-outbox').objectStore('capture-outbox').getAll();
        request.onsuccess = () => resolve(request.result);
        request.onerror = () => reject(request.error);
      };
    }));
    const offlineItems = await readOutbox();
    assert(offlineItems.length === 1 && offlineItems[0].content === offlineNote && offlineItems[0].episode_id === episodeId, 'offline note was not durable in IndexedDB');

    await page.context().setOffline(false);
    await page.evaluate(() => window.dispatchEvent(new Event('online')));
    let syncState = { outboxCount: -1, noteCount: -1 };
    for (let attempt = 0; attempt < 80; attempt += 1) {
      syncState = await page.evaluate(async ({ id, text }) => {
        const outboxCount = await new Promise((resolve, reject) => {
          const open = indexedDB.open('echonote', 1);
          open.onerror = () => reject(open.error);
          open.onsuccess = () => {
            const request = open.result.transaction('capture-outbox').objectStore('capture-outbox').count();
            request.onsuccess = () => resolve(request.result);
            request.onerror = () => reject(request.error);
          };
        });
        try {
          const response = await fetch('/api/v1/episodes/' + encodeURIComponent(id) + '/notes');
          const body = response.ok ? await response.json() : { items: [] };
          return { outboxCount, noteCount: body.items.filter(note => note.content === text).length };
        } catch {
          return { outboxCount, noteCount: -1 };
        }
      }, { id: episodeId, text: offlineNote });
      if (syncState.outboxCount === 0 && syncState.noteCount === 1) break;
      await page.waitForTimeout(250);
    }
    assert(syncState.outboxCount === 0 && syncState.noteCount === 1, 'outbox sync state is ' + JSON.stringify(syncState));

    await page.goto(origin + '/library');
    const importURLs = [
      'https://podcasts.apple.com/us/podcast/browser-smoke/id123456789?i=1000000001',
      'https://feeds.example.com/$username.xml',
      'https://cdn.example.com/$username.mp3'
    ];
    for (const url of importURLs) {
      await page.getByRole('button', { name: '导入节目' }).first().click();
      const dialog = page.getByRole('dialog', { name: '导入节目' });
      await dialog.getByRole('textbox', { name: '节目链接' }).fill(url);
      await dialog.getByRole('button', { name: '导入节目' }).click();
      await dialog.waitFor({ state: 'hidden' });
    }
    assert(importPosts.length === 3 && importURLs.every(url => importPosts.includes(url)), 'three import sources did not reach the real API');

    const deleteStatus = await page.evaluate(async id =>
      (await fetch('/api/v1/episodes/' + encodeURIComponent(id), { method: 'DELETE' })).status,
      episodeId
    );
    assert(deleteStatus === 204, 'episode cleanup failed');
    episodeId = '';
    await page.goto(origin + '/mine');
    await page.getByRole('button', { name: '退出并保留待发送记录' }).click();
    await page.waitForURL('**/login');
    const logoutStatus = await page.evaluate(async () => (await fetch('/api/v1/me')).status);
    assert(logoutStatus === 401, 'logout did not revoke the session');
    return {
      imports: importPosts.length, transcriptionSSE, speakers: 1, searchTypes,
      aiSSE: true, exports: 4, offlineOutbox: true, logoutStatus
    };
  } finally {
    await page.context().setOffline(false).catch(() => {});
    if (episodeId) {
      await page.evaluate(async id => {
        await fetch('/api/v1/episodes/' + encodeURIComponent(id), { method: 'DELETE' });
      }, episodeId).catch(() => {});
    }
  }
}
PLAYWRIGHT

set +e
smoke_result="$(pwcli --raw run-code "$smoke_code")"
smoke_status=$?
set -e
printf '%s\n' "$smoke_result"
[[ "$smoke_status" -eq 0 ]] || exit "$smoke_status"
for marker in '"imports":3' '"resumed":["completed"]' '"speakers":1' '"searchTypes":["ai_artifact","note","transcript"]' '"aiSSE":true' '"exports":4' '"offlineOutbox":true' '"logoutStatus":401'; do
  [[ "$smoke_result" == *"$marker"* ]] || {
    echo "browser smoke did not return required marker: $marker" >&2
    exit 1
  }
done

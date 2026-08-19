import type { AiSummary, Episode, Note, SearchResultItem, Speaker, TranscriptSegment } from '../types'
import { baseTranscriptSegments, fourSpeakerExtraSegments } from './transcript'
import { useTestMode, type TestModeState } from '../store/test-mode'
import { useCaptureStore } from '../store/capture'

export const statusLabels: Record<Episode['status'], string> = {
  transcribed: '已转录',
  transcribing: '转录中',
  waiting: '等待转录',
  failed: '转录失败'
}

export function statusTextClass(status: Episode['status']): string {
  switch (status) {
    case 'transcribed':
      return 'text-ink-tertiary'
    case 'transcribing':
      return 'text-accent'
    case 'waiting':
      return 'text-ink-tertiary'
    case 'failed':
      return 'text-danger'
  }
}

const fewNotes: Note[] = [
  {
    id: 'n1',
    createdAt: '19:32',
    text: '这里 FDE 的定义和我之前理解的不一样'
  },
  {
    id: 'n2',
    createdAt: '19:45',
    text: '260 个步骤这个案例值得重点整理'
  },
  {
    id: 'n3',
    createdAt: '20:03',
    text: '后面可以问一下 AI：中国式 FDE 和美国模式有什么区别？'
  }
]

const manyNotes: Note[] = [
  ...fewNotes,
  {
    id: 'n4',
    createdAt: '20:12',
    text: '“失败之后能不能被理解”这句话说得很好，信任是一点点积累的。'
  },
  {
    id: 'n5',
    createdAt: '20:24',
    text: 'Agent 不是全自动，关键要知道什么时候停下来问人。'
  },
  {
    id: 'n6',
    createdAt: '20:31',
    text: '企业买的不是聊天窗口，是结果。这句可以作为核心观点。'
  },
  {
    id: 'n7',
    createdAt: '20:42',
    text: '想整理一个对比：传统 RPA、Chatbot、企业 Agent 之间的边界。'
  },
  {
    id: 'n8',
    createdAt: '20:55',
    text: '语言风格也是业务能力。AI 消息像机器人会让用户以为是诈骗，这个细节值得记住。'
  },
  {
    id: 'n9',
    createdAt: '21:06',
    text: 'FDE 要下现场。把隐性知识显性化，可能比模型能力更重要。'
  },
  {
    id: 'n10',
    createdAt: '21:18',
    text: '下次可以和团队讨论：最小闭环应该先选哪个流程。'
  }
]

const baseEpisodes = [
  {
    id: 'e1',
    showTitle: '硅谷101',
    episodeTitle: 'E248｜一个“催发货”AI要跑通260步，和阿里瓴羊朋新宇聊聊中国式FDE',
    episodeTitleLong: 'E248｜一个“催发货”AI要跑通260步，和阿里瓴羊朋新宇聊聊中国式FDE',
    episodeTitleShort: 'E248｜AI 如何跑通 260 步',
    durationMin: 64,
    baseStatus: 'transcribed' as const,
    recordedLabel: '今天',
    coverLabel: '硅',
    transcriptAvailable: true,
    aiAvailable: true
  },
  {
    id: 'e2',
    showTitle: '原点 The Origin',
    episodeTitle: '钦文对话赵凯：23岁在硅谷融资千万美金',
    episodeTitleLong: '钦文对话赵凯：23岁在硅谷融资千万美金之后，他如何理解机会、运气与长期主义',
    episodeTitleShort: '钦文对话赵凯',
    durationMin: 42,
    baseStatus: 'transcribing' as const,
    recordedLabel: '昨天',
    coverLabel: '原',
    transcriptAvailable: false,
    aiAvailable: false
  },
  {
    id: 'e3',
    showTitle: '声动早咖啡',
    episodeTitle: '从硅谷到杭州：这一轮 Agent 创业为什么先做落地？',
    episodeTitleLong: '从硅谷到杭州：这一轮 Agent 创业为什么先做落地，而不是先做通用助手？',
    episodeTitleShort: 'Agent 创业为什么先做落地',
    durationMin: 18,
    baseStatus: 'waiting' as const,
    recordedLabel: '周二',
    coverLabel: '声',
    transcriptAvailable: false,
    aiAvailable: false
  },
  {
    id: 'e4',
    showTitle: '晚点聊 LateTalk',
    episodeTitle: '大模型之后，SaaS 会变成什么？',
    episodeTitleLong: '大模型之后，SaaS 会变成什么？从按席收费到按结果付费，中间还隔着多少工程问题',
    episodeTitleShort: 'SaaS 会变成什么',
    durationMin: 71,
    baseStatus: 'failed' as const,
    recordedLabel: '6月12日',
    coverLabel: '晚',
    transcriptAvailable: false,
    aiAvailable: false
  }
]

function notesForEpisode(id: string, density: TestModeState['notesDensity']): Note[] {
  if (id === 'e1') {
    if (density === 'none') return []
    if (density === 'many') return manyNotes
    return fewNotes
  }
  if (id === 'e2') {
    return [
      {
        id: 'e2n1',
        createdAt: '08:14',
        text: '赵凯讲到融资节奏的部分值得回看，机会判断那一段很清醒。'
      }
    ]
  }
  if (id === 'e4') {
    return [
      {
        id: 'e4n1',
        createdAt: '11:05',
        text: '按结果付费会改变 SaaS 的交付方式，这个判断可以继续观察。'
      },
      {
        id: 'e4n2',
        createdAt: '11:22',
        text: '软件公司以后可能需要更像服务公司。'
      }
    ]
  }
  return []
}

function buildEpisodes(state: TestModeState, extraNotes: Record<string, Note[]>): Episode[] {
  return baseEpisodes.map((base) => {
    const isPrimary = base.id === 'e1'
    const status = isPrimary ? state.primaryStatus : base.baseStatus
    const long = state.titleLength === 'long'
    return {
      id: base.id,
      showTitle: base.showTitle,
      episodeTitle: long ? base.episodeTitle : base.episodeTitleShort,
      episodeTitleLong: long ? base.episodeTitleLong : base.episodeTitleShort,
      durationMin: base.durationMin,
      status,
      recordedLabel: base.recordedLabel,
      notes: [...notesForEpisode(base.id, state.notesDensity), ...(extraNotes[base.id] ?? [])],
      coverLabel: base.coverLabel,
      transcriptAvailable: isPrimary && status === 'transcribed',
      aiAvailable: isPrimary && status === 'transcribed'
    }
  })
}

export function useEpisodes(): Episode[] {
  const state = useTestMode()
  const extraNotes = useCaptureStore((s) => s.extraNotes)
  return buildEpisodes(state, extraNotes)
}

export function getEpisodesSnapshot(): Episode[] {
  return buildEpisodes(useTestMode.getState(), useCaptureStore.getState().extraNotes)
}

export function getEpisode(id: string): Episode | undefined {
  return getEpisodesSnapshot().find((episode) => episode.id === id)
}

export function useEpisode(id: string): Episode | undefined {
  const episodes = useEpisodes()
  return episodes.find((episode) => episode.id === id)
}

const speakers: Speaker[] = [
  { id: 'host', name: '主持人' },
  { id: 'peng', name: '朋新宇' },
  { id: 'guest', name: '李然' },
  { id: 'audience', name: '现场提问' }
]

function timestampToSeconds(timestamp: string): number {
  const parts = timestamp.split(':').map(Number)
  return parts[0] * 3600 + parts[1] * 60 + (parts[2] ?? 0)
}

export function getSpeaker(speakerId: Speaker['id']): Speaker {
  return speakers.find((speaker) => speaker.id === speakerId) ?? speakers[0]
}

export function useTranscript(episodeId: string): TranscriptSegment[] {
  const state = useTestMode()
  if (episodeId !== 'e1') return []

  let segments =
    state.speakerCount === 4
      ? [...baseTranscriptSegments, ...fourSpeakerExtraSegments].sort(
          (a, b) => timestampToSeconds(a.timestamp) - timestampToSeconds(b.timestamp)
        )
      : [...baseTranscriptSegments]

  if (state.speakerCount === 1) {
    segments = segments.map((segment) => ({ ...segment, speakerId: 'peng' as const }))
  }

  return state.transcriptLength === 'small' ? segments.slice(0, 6) : segments
}

function summarizeSnippet(text: string, query: string, maxLength = 120): string {
  const normalized = text.toLocaleLowerCase('zh-CN')
  const index = normalized.indexOf(query.toLocaleLowerCase('zh-CN'))
  if (index < 0) {
    return text.length > maxLength ? `${text.slice(0, maxLength)}…` : text
  }
  const start = Math.max(0, index - 32)
  const end = Math.min(text.length, index + query.length + 84)
  const prefix = start > 0 ? '…' : ''
  const suffix = end < text.length ? '…' : ''
  return `${prefix}${text.slice(start, end)}${suffix}`
}

export function useSearchResults(query: string): SearchResultItem[] {
  const episodes = useEpisodes()
  const transcript = useTranscript('e1')
  const q = query.trim()

  if (q.length < 2) return []

  const results: SearchResultItem[] = []
  const target = q.toLocaleLowerCase('zh-CN')
  const primary = episodes[0]

  if (primary) {
    for (const note of primary.notes) {
      if (note.text.toLocaleLowerCase('zh-CN').includes(target)) {
        results.push({
          kind: 'note',
          episodeId: primary.id,
          episodeTitle: primary.episodeTitle,
          showTitle: primary.showTitle,
          snippet: summarizeSnippet(note.text, q),
          meta: note.createdAt
        })
      }
    }
  }

  if (primary?.transcriptAvailable) {
    for (const segment of transcript) {
      if (segment.text.toLocaleLowerCase('zh-CN').includes(target)) {
        const speaker = getSpeaker(segment.speakerId)
        results.push({
          kind: 'transcript',
          episodeId: 'e1',
          episodeTitle: primary?.episodeTitle ?? '',
          showTitle: primary?.showTitle ?? '硅谷101',
          snippet: summarizeSnippet(segment.text, q),
          meta: `${speaker.name} · ${segment.timestamp}`
        })
      }
    }
  }

  const summary = buildAiSummary(primary)
  if (primary?.aiAvailable) {
    const aiTexts = [summary.oneLiner, ...summary.corePoints, ...summary.viewpoints.map((v) => v.point)]
    if (aiTexts.some((text) => text.toLocaleLowerCase('zh-CN').includes(target))) {
      const hit = aiTexts.find((text) => text.toLocaleLowerCase('zh-CN').includes(target)) ?? summary.oneLiner
      results.push({
        kind: 'ai',
        episodeId: primary?.id ?? 'e1',
        episodeTitle: primary?.episodeTitle ?? '',
        showTitle: primary?.showTitle ?? '硅谷101',
        snippet: summarizeSnippet(hit, q),
        meta: 'AI 整理'
      })
    }
  }

  return results.slice(0, 12)
}

export function buildAiSummary(episode: Episode | undefined): AiSummary {
  const noteConnections =
    episode && episode.notes.length > 0
      ? episode.notes.slice(0, 3).map((note, index) => ({
          note: note.text,
          insight: [
            '这条笔记正好落在“FDE 的定义与边界”这个主题上，节目后半段给出了更具体的解释。',
            '可以把这条观察和“260 步的异常恢复”放在一起看，它们都指向企业 Agent 的真实复杂度。',
            '这个问题适合继续追问：节目给出的答案偏向现场视角，还可以补充更多美国市场的对照案例。'
          ][index]
        }))
      : [
          {
            note: '本期还没有你的笔记。',
            insight: '听完后随手记录几个想法，AI 整理会把它们和节目内容放在同一张地图里对照。'
          }
        ]

  return {
    oneLiner:
      '这期节目讨论的不是模型本身，而是企业 AI 落地的最后一公里：FDE 如何把复杂业务流程拆成可执行、可恢复、可被理解的 Agent。',
    corePoints: [
      'FDE 的核心不是简单部署模型，而是深入企业业务流程，把隐性知识显性化。',
      '真正复杂的企业 Agent 可能需要完成数百个步骤，难点往往不在步骤数量，而在异常分支与失败恢复。',
      '中国企业的软件生态更碎片化，因此中国式 FDE 会更重现场沟通与系统对接。',
      '可信赖比全自动更重要：涉及金额、合同与对外承诺时，Agent 必须学会停下来问人。'
    ],
    viewpoints: [
      {
        speaker: '朋新宇',
        point: '企业 AI 的模型之外工程占九成；Demo 很漂亮不是结果，接住真实数据才是结果。'
      },
      {
        speaker: '朋新宇',
        point: '最小闭环的意义不是省人力，而是让团队看到机器真的能接住一部分重复决策，从而积累信任。'
      },
      {
        speaker: '主持人',
        point: '衡量 Agent 不能只看成功率，还要看失败时能否被理解，以及它是否懂得何时不自动。'
      }
    ],
    worthReviewing: [
      {
        timestamp: '00:14:44',
        quote: '全自动不是目标，可信赖才是目标。',
        reason: '一句话概括了企业 Agent 的边界原则，适合作为之后讨论的锚点。'
      },
      {
        timestamp: '00:16:29',
        quote: '中国的企业软件生态更碎片化，大量系统是后来长出来的。',
        reason: '解释了为什么中国式 FDE 更“重”，是理解中美差异的关键段落。'
      },
      {
        timestamp: '00:26:40',
        quote: '最初我们以为最难的是流程长，后来发现最难的是异常恢复。',
        reason: '来自 260 步案例的一线经验，适合结合笔记反复回顾。'
      },
      {
        timestamp: '00:32:46',
        quote: '语言风格本身也是业务能力。',
        reason: '一个容易被忽视但非常重要的产品细节。'
      }
    ],
    noteConnections
  }
}

export function useAiSummary(episodeId: string): AiSummary {
  const episode = useEpisode(episodeId)
  return buildAiSummary(episode)
}

(function () {
  try {
    var saved = localStorage.getItem('echonote.theme')
    var mode = saved ? JSON.parse(saved).state.theme : 'system'
    var dark = mode === 'dark' || (mode === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches)
    if (dark) document.documentElement.classList.add('dark')
    var color = dark ? '#15120e' : '#f5f3ee'
    var meta = document.querySelector('meta[name="theme-color"]')
    if (meta) meta.setAttribute('content', color)
    var manifest = document.getElementById('pwa-manifest')
    if (manifest) manifest.setAttribute('href', dark ? '/manifest-dark.webmanifest' : '/manifest.webmanifest')
    var statusBar = document.querySelector('meta[name="apple-mobile-web-app-status-bar-style"]')
    if (statusBar) statusBar.setAttribute('content', dark ? 'black-translucent' : 'default')
    if (mode !== 'system') {
      var splashLinks = document.querySelectorAll('link[rel="apple-touch-startup-image"]')
      for (var i = 0; i < splashLinks.length; i++) {
        var link = splashLinks[i]
        var href = link.getAttribute('href') || ''
        var originalMedia = link.getAttribute('data-original-media') || link.getAttribute('media') || ''
        var isDarkImage = href.indexOf('/apple-splash-dark-') !== -1
        var isWanted = dark ? isDarkImage : !isDarkImage
        link.setAttribute(
          'media',
          isWanted
            ? originalMedia.replace('(prefers-color-scheme: light) and ', '').replace('(prefers-color-scheme: dark) and ', '')
            : '(min-width: 100000px)'
        )
      }
    }
    document.documentElement.style.colorScheme = dark ? 'dark' : 'light'
  } catch (error) {
    // Invalid local preferences fall back to the document defaults.
  }
})()

// Set the theme before first paint so dark-mode users never see a light flash. This is a separate
// classic script (not inline) so the page can be served with a Content-Security-Policy that forbids
// inline scripts. It reads the same localStorage key as src/state/persist.ts.
;(function () {
  var theme = null
  try {
    theme = (JSON.parse(localStorage.getItem('sdp:v1') || 'null') || {}).theme
  } catch (e) {
    theme = null // storage blocked or corrupt: follow the system
  }
  if (theme !== 'light' && theme !== 'dark') theme = matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  document.documentElement.setAttribute('data-theme', theme)
})()

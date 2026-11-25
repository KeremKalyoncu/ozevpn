(function () {
  var saved = null;
  try {
    saved = localStorage.getItem('ozevpn_theme');
  } catch (e) {}
  var theme = (saved === 'light' || saved === 'dark')
    ? saved
    : (window.matchMedia && window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark');
  document.documentElement.dataset.theme = theme;
})();

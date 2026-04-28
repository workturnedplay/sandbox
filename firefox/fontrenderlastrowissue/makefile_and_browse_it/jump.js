(() => {
  const targetVisualLine = 729444; // last good line; 729445 is borked
  const lineHeightPx = 15.333328247070312;
  const pre = document.querySelector('pre');
  const preAbsY = pre.getBoundingClientRect().top + window.scrollY;

  const targetAbsY = preAbsY + (targetVisualLine - 1) * lineHeightPx;
  window.scrollTo({ top: targetAbsY - window.innerHeight * 0.4, behavior: 'instant' });

  console.log('scrolled to abs Y:', targetAbsY);
})();
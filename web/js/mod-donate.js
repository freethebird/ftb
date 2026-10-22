/* mod-donate — RPC node donation (static, no chain, no keys).
   One root (#donateRoot); self-guards; copy-to-clipboard for the XMR address.
   CSP-safe: external script, no inline handlers. */
(function () {
  'use strict';
  var root = document.getElementById('donateRoot');
  if (!root) return;                         // R2: install nothing if the panel is absent
  var btn = document.getElementById('xmrCopy');
  var addrEl = document.getElementById('xmrAddr');
  if (!btn || !addrEl) return;
  var addr = (addrEl.textContent || '').trim();
  var reset;

  function flash(txt, cls) {
    btn.textContent = txt;
    if (cls) btn.classList.add(cls);
    clearTimeout(reset);
    reset = setTimeout(function () {
      btn.textContent = 'copy address';
      btn.classList.remove('copied');
    }, 1800);
  }
  function selectAddr() {
    try {
      var r = document.createRange(); r.selectNodeContents(addrEl);
      var sel = window.getSelection(); sel.removeAllRanges(); sel.addRange(r);
      return sel;
    } catch (e) { return null; }
  }
  btn.addEventListener('click', function () {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(addr).then(
        function () { flash('copied ✓', 'copied'); },
        function () { var s = selectAddr(); try { document.execCommand('copy'); } catch (e) {} if (s) s.removeAllRanges(); flash('copied ✓', 'copied'); }
      );
    } else {
      var s = selectAddr(); try { document.execCommand('copy'); } catch (e) {} if (s) s.removeAllRanges();
      flash('copied ✓', 'copied');
    }
  });
})();

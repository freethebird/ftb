/* mod-donate — static donation cards (no chain, no keys): copy-to-clipboard for each
   address (GAS main-net launch fund + XMR node donation). CSP-safe: external script,
   no inline handlers. */
(function () {
  'use strict';
  var root = document.getElementById('donateRoot');
  if (!root) return;                         // install nothing if the panel is absent

  function wire(btnId, addrId) {
    var btn = document.getElementById(btnId);
    var addrEl = document.getElementById(addrId);
    if (!btn || !addrEl) return;             // skip a card that is not on this page
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
  }

  wire('gasCopy', 'gasAddr');
  wire('xmrCopy', 'xmrAddr');
})();

(function () {
  if (window.__protopenCommentsLoaded) return;
  window.__protopenCommentsLoaded = true;

  var state = { mode: 'comment', pins: [] };
  var root = document.createElement('div');
  root.id = '__protopen_pins';
  root.style.cssText = 'position:absolute;top:0;left:0;pointer-events:none;z-index:2147483646';
  document.body.appendChild(root);

  function post(msg) { try { window.parent.postMessage(msg, '*'); } catch (e) {} }
  function dims() { return { w: document.documentElement.scrollWidth, h: document.documentElement.scrollHeight }; }

  function renderPins() {
    root.innerHTML = '';
    var d = dims();
    state.pins.forEach(function (pin, i) {
      var el = document.createElement('div');
      el.dataset.commentId = pin.id;
      el.style.cssText = 'position:absolute;width:24px;height:24px;border-radius:50% 50% 50% 0;background:#ff8f52;color:white;font:600 12px/24px system-ui,sans-serif;text-align:center;transform:translate(-12px,-24px) rotate(-45deg);box-shadow:0 2px 8px rgba(0,0,0,.25);pointer-events:auto;cursor:pointer;';
      el.style.left = (pin.x * d.w) + 'px';
      el.style.top = (pin.y * d.h) + 'px';
      var inner = document.createElement('span');
      inner.style.cssText = 'display:block;transform:rotate(45deg)';
      inner.textContent = String(pin.seq || i + 1);
      el.appendChild(inner);
      el.addEventListener('click', function (e) {
        e.stopPropagation();
        post({ type: 'protopen-pin-click', id: pin.id });
      });
      root.appendChild(el);
    });
  }

  function highlight(id) {
    var el = root.querySelector('[data-comment-id="' + id + '"]');
    if (!el) return;
    el.scrollIntoView({ block: 'center', behavior: 'smooth' });
    el.style.outline = '3px solid #0066ff';
    setTimeout(function () { el.style.outline = ''; }, 1500);
  }

  document.addEventListener('click', function (e) {
    if (state.mode !== 'comment') return;
    if (e.target.closest && e.target.closest('a[href]')) return;
    if (e.target.closest && e.target.closest('#__protopen_pins')) return;
    var d = dims();
    var x = (e.pageX || (e.clientX + window.scrollX)) / d.w;
    var y = (e.pageY || (e.clientY + window.scrollY)) / d.h;
    if (x < 0 || x > 1 || y < 0 || y > 1) return;
    e.preventDefault();
    post({ type: 'protopen-click', x: x, y: y, pagePath: location.pathname });
  }, true);

  window.addEventListener('message', function (e) {
    if (e.source !== window.parent) return;
    var msg = e.data || {};
    if (msg.type === 'protopen-set-pins') {
      state.pins = msg.pins || [];
      renderPins();
    } else if (msg.type === 'protopen-set-mode') {
      state.mode = msg.mode === 'browse' ? 'browse' : 'comment';
    } else if (msg.type === 'protopen-highlight-pin') {
      highlight(msg.id);
    }
  });

  var lastPath = location.pathname;
  function notifyNavigate() {
    var p = location.pathname;
    if (p !== lastPath) {
      lastPath = p;
      post({ type: 'protopen-navigate', path: p });
    }
  }
  ['pushState', 'replaceState'].forEach(function (m) {
    var orig = history[m];
    history[m] = function () { var r = orig.apply(this, arguments); notifyNavigate(); return r; };
  });
  window.addEventListener('popstate', notifyNavigate);

  post({ type: 'protopen-ready', path: location.pathname, dims: dims() });
})();

(function () {
  if (window.__protopenCommentsLoaded) return;
  window.__protopenCommentsLoaded = true;

  // ---------- config from <script data-*> attributes ----------
  var script = document.currentScript || (function () {
    var scripts = document.getElementsByTagName('script');
    for (var i = scripts.length - 1; i >= 0; i--) {
      if (scripts[i].src && scripts[i].src.indexOf('comment-runtime.js') !== -1) return scripts[i];
    }
    return null;
  })();

  var dataset = (script && script.dataset) || {};
  var orgSlug = dataset.siteOrg || deriveSlugFromPath(0);
  var siteSlug = dataset.siteSlug || deriveSlugFromPath(1);
  var apiBase = (dataset.apiBase || '').replace(/\/$/, '');
  var servedDeployId = dataset.deployId || '';
  if (!orgSlug || !siteSlug) return;

  function deriveSlugFromPath(index) {
    var parts = location.pathname.split('/').filter(Boolean);
    if (!parts.length || parts[0].charAt(0) !== '~') return '';
    if (index === 0) return parts[0].slice(1);
    return parts[1] || '';
  }

  var TOPBAR_H = 40;

  // ---------- state ----------
  var state = {
    mode: 'browse',          // 'browse' | 'comment'
    comments: [],            // server-side list, scoped to site
    siteId: null,
    deployId: null,
    isPublic: false,
    siteName: '',
    user: null,              // signed-in session user, if any
    pendingPin: null,        // captured but not yet posted
    activeThreadId: null,    // currently-open thread popover
    mentionCandidates: [],
  };

  // ---------- shadow root + chrome ----------
  var host = document.createElement('div');
  host.id = '__protopen_host';
  host.style.cssText = 'position:fixed;inset:0;pointer-events:none;z-index:2147483646';
  document.documentElement.appendChild(host);

  // Shift the page content down so the fixed topbar doesn't cover the site's
  // own nav. Use padding-top, not margin-top: body's margin-top collapses
  // through to <html> and won't actually move static-positioned content.
  function applyBodyShift() {
    if (!document.body) return;
    if (document.body.dataset.protopenShifted) return;
    var existing = parseFloat(getComputedStyle(document.body).paddingTop) || 0;
    document.body.style.paddingTop = (existing + TOPBAR_H) + 'px';
    document.body.dataset.protopenShifted = '1';
  }
  if (document.body) applyBodyShift();
  else document.addEventListener('DOMContentLoaded', applyBodyShift);
  var shadow = host.attachShadow({ mode: 'open' });

  shadow.innerHTML = (
    '<style>' +
    ':host, * { box-sizing: border-box; font-family: system-ui, -apple-system, "Segoe UI", sans-serif; }' +
    '.pin-layer { position: absolute; inset: 0; pointer-events: none; }' +
    '.pin { position: absolute; width: 28px; height: 28px; border-radius: 50% 50% 50% 0; background: rgba(255,143,82,.92); color: white; font: 600 12px system-ui, sans-serif; --pin-scale: 1; transform-origin: 0% 100%; transform: translate(0, -100%) rotate(-45deg) scale(var(--pin-scale)); box-shadow: 0 2px 8px rgba(0,0,0,.25); border: 2px solid rgba(255,255,255,.6); -webkit-backdrop-filter: blur(8px) saturate(1.4); backdrop-filter: blur(8px) saturate(1.4); pointer-events: auto; cursor: pointer; display: flex; align-items: center; justify-content: center; user-select: none; transition: transform .15s, filter .15s; }' +
    '.pin:hover { --pin-scale: 1.1; filter: brightness(1.05); }' +
    '.pin > span { transform: rotate(45deg); line-height: 1; }' +
    '.pin.unanchored { border: 2px dashed rgba(255,255,255,.6); }' +
    '.pin.active { background: #0066ff; }' +
    '.pin.draggable { cursor: grab; }' +
    '.pin.dragging { --pin-scale: 1.15; opacity: .75; cursor: grabbing; transition: none; z-index: 2147483646; }' +
    '.topbar { position: fixed; top: 0; left: 0; right: 0; height: ' + TOPBAR_H + 'px; background: #111; color: white; display: flex; align-items: center; justify-content: space-between; padding: 0 16px; font: 500 13px system-ui; box-shadow: 0 1px 4px rgba(0,0,0,.25); pointer-events: auto; z-index: 1; }' +
    '.topbar .brand { display: flex; align-items: center; gap: 8px; color: inherit; text-decoration: none; opacity: .7; font-size: 12px; letter-spacing: .02em; text-transform: uppercase; transition: opacity .15s; }' +
    '.topbar .brand:hover { opacity: 1; }' +
    '.topbar .brand::before { content: ""; display: inline-block; width: 10px; height: 10px; border-radius: 50% 50% 50% 0; background: #ff8f52; transform: rotate(-45deg); }' +
    '.topbar .toggle-btn { display: inline-flex; align-items: center; gap: 8px; background: transparent; color: inherit; border: 1px solid rgba(255,255,255,.2); padding: 6px 14px; border-radius: 999px; cursor: pointer; font: 500 13px system-ui; transition: background .15s, border-color .15s; }' +
    '.topbar .toggle-btn:hover { border-color: rgba(255,255,255,.4); }' +
    '.topbar .toggle-btn.active { background: #ff8f52; border-color: #ff8f52; }' +
    '.topbar .toggle-btn .count { display: inline-flex; min-width: 18px; height: 18px; padding: 0 6px; align-items: center; justify-content: center; background: rgba(255,255,255,.22); border-radius: 999px; font-size: 11px; }' +
    '.topbar .toggle-btn .count:empty { display: none; }' +
    '.topbar .shortcut { opacity: .55; font-size: 11px; margin-left: 4px; }' +
    '.topbar-right { display: flex; align-items: center; gap: 10px; }' +
    '.topbar .versions { appearance: none; -webkit-appearance: none; background: transparent; border: 1px solid rgba(255,255,255,.2); color: inherit; font: 500 12px system-ui; padding: 5px 24px 5px 12px; border-radius: 999px; cursor: pointer; background-image: url("data:image/svg+xml;utf8,<svg xmlns=\'http://www.w3.org/2000/svg\' width=\'8\' height=\'8\' viewBox=\'0 0 8 8\'><path d=\'M2 3l2 2 2-2\' stroke=\'white\' stroke-width=\'1\' fill=\'none\'/></svg>"); background-repeat: no-repeat; background-position: right 8px center; outline: none; }' +
    '.topbar .versions:hover { border-color: rgba(255,255,255,.4); }' +
    '.topbar .versions option { background: #111; color: white; }' +
    '.composer { position: fixed; background: rgba(255,255,255,.92); -webkit-backdrop-filter: blur(20px) saturate(1.5); backdrop-filter: blur(20px) saturate(1.5); color: #18181b; border: 1px solid #e4e4e7; border-radius: 20px; box-shadow: 0 4px 24px rgba(0,0,0,.12); padding: 6px 6px 6px 14px; pointer-events: auto; z-index: 2; min-width: 260px; max-width: 360px; display: flex; align-items: flex-end; gap: 6px; }' +
    '.composer textarea { flex: 1; border: 0; background: transparent; padding: 8px 0; font: 13px system-ui; resize: none; outline: none; min-height: 18px; max-height: 120px; overflow-y: auto; line-height: 1.4; }' +
    '.composer .send { width: 30px; height: 30px; border-radius: 50%; border: 0; background: #e4e4e7; color: #a1a1aa; display: flex; align-items: center; justify-content: center; cursor: pointer; flex-shrink: 0; transition: background .15s, color .15s; }' +
    '.composer .send.active { background: #ff8f52; color: white; }' +
    '.composer .signin { flex: 1; font-size: 13px; color: #444; padding: 8px 0; line-height: 1.4; }' +
    '.composer .signin a { color: #ff8f52; font-weight: 600; text-decoration: none; }' +
    '.composer .signin a:hover { text-decoration: underline; }' +
    '.composer .close { width: 28px; height: 28px; border-radius: 50%; border: 0; background: transparent; color: #a1a1aa; display: flex; align-items: center; justify-content: center; cursor: pointer; flex-shrink: 0; }' +
    '.composer .close:hover { color: #71717a; background: rgba(0,0,0,.04); }' +
    '.popover { position: absolute; pointer-events: auto; background: rgba(255,255,255,.92); -webkit-backdrop-filter: blur(20px) saturate(1.5); backdrop-filter: blur(20px) saturate(1.5); color: #18181b; border: 1px solid #e4e4e7; border-radius: 10px; box-shadow: 0 4px 24px rgba(0,0,0,.1); width: 320px; font-size: 13px; overflow: hidden; z-index: 2; }' +
    '.pop-toolbar { display: flex; align-items: center; gap: 2px; position: relative; margin-left: auto; }' +
    '.pop-toolbar button { width: 26px; height: 26px; border-radius: 50%; border: 0; background: transparent; cursor: pointer; display: flex; align-items: center; justify-content: center; color: #71717a; padding: 0; transition: color .12s, background .12s; }' +
    '.pop-toolbar button:hover { color: #18181b; background: rgba(0,0,0,.05); }' +
    '.pop-toolbar button.on { color: #18181b; }' +
    '.pop-menu { position: absolute; top: 100%; right: 0; margin-top: 4px; background: #18181b; border-radius: 8px; padding: 4px 0; min-width: 160px; box-shadow: 0 6px 20px rgba(0,0,0,.25); z-index: 4; }' +
    '.pop-menu button { display: block; width: 100%; text-align: left; padding: 7px 14px; background: transparent; border: 0; color: white; font: 500 12px system-ui; cursor: pointer; border-radius: 0; }' +
    '.pop-menu button:hover { background: rgba(255,255,255,.1); color: white; }' +
    '.pop-menu .copied { color: #a3e635; }' +
    '.pop-body { padding: 12px 14px; max-height: 320px; overflow-y: auto; }' +
    '.msg { margin-bottom: 10px; }' +
    '.msg:last-child { margin-bottom: 0; }' +
    '.msg-head { display: flex; align-items: center; gap: 6px; margin-bottom: 3px; }' +
    '.avatar { width: 22px; height: 22px; border-radius: 50%; background: #18181b; color: white; font-size: 10px; font-weight: 700; display: flex; align-items: center; justify-content: center; flex-shrink: 0; }' +
    '.msg-author { font-weight: 600; font-size: 12px; }' +
    '.msg-time { color: #a1a1aa; font-size: 11px; }' +
    '.msg-body { line-height: 1.5; white-space: pre-wrap; word-break: break-word; }' +
    '.replies { border-top: 1px solid #f4f4f5; margin-top: 8px; padding-top: 8px; }' +
    '.reply-msg { margin-bottom: 8px; padding-left: 10px; border-left: 2px solid #e4e4e7; }' +
    '.reply-msg:last-child { margin-bottom: 0; }' +
    '.pop-compose { border-top: 1px solid #e4e4e7; padding: 8px 10px; display: flex; align-items: center; gap: 6px; }' +
    '.pop-compose textarea { flex: 1; border: 0; background: transparent; padding: 6px 0; font-family: inherit; font-size: 12px; resize: none; outline: none; min-height: 28px; max-height: 80px; overflow-y: auto; line-height: 16px; box-sizing: border-box; }' +
    '.pop-compose .send { width: 28px; height: 28px; border-radius: 50%; background: #e4e4e7; color: #a1a1aa; border: 0; display: flex; align-items: center; justify-content: center; cursor: pointer; flex-shrink: 0; transition: background .15s, color .15s; }' +
    '.pop-compose .send.active { background: #ff8f52; color: white; }' +
    '.mentions { position: absolute; background: white; border: 1px solid #ddd; border-radius: 6px; box-shadow: 0 4px 12px rgba(0,0,0,.12); margin-top: 2px; max-height: 200px; overflow-y: auto; min-width: 200px; z-index: 3; }' +
    '.mentions div { padding: 6px 10px; cursor: pointer; font-size: 13px; }' +
    '.mentions div:hover, .mentions div.selected { background: #f0f7ff; }' +
    '</style>' +
    '<div class="pin-layer"></div>' +
    '<div class="topbar">' +
      '<a class="brand" href="' + escapeHTML(apiBase || '/') + '" target="_blank" rel="noopener">Protopen</a>' +
      '<div class="topbar-right">' +
        '<button class="toggle-btn" aria-pressed="false">' +
          '<span class="label">Comments</span>' +
          '<span class="count"></span>' +
          '<span class="shortcut">C</span>' +
        '</button>' +
      '</div>' +
    '</div>'
  );

  var pinLayer = shadow.querySelector('.pin-layer');
  var toggleBtn = shadow.querySelector('.toggle-btn');
  var countBadge = shadow.querySelector('.toggle-btn .count');
  var topbarRight = shadow.querySelector('.topbar-right');

  // Cursor style for the host page when in comment mode. Single <style> in
  // <head> is the only host-page mutation aside from #__protopen_host.
  var cursorStyle = document.createElement('style');
  cursorStyle.id = '__protopen_cursor';
  cursorStyle.textContent = '';
  document.head.appendChild(cursorStyle);

  // ---------- mode + chrome interactions ----------
  function setMode(mode) {
    state.mode = mode === 'comment' ? 'comment' : 'browse';
    var on = state.mode === 'comment';
    toggleBtn.classList.toggle('active', on);
    toggleBtn.setAttribute('aria-pressed', on ? 'true' : 'false');
    cursorStyle.textContent = on ? 'body { cursor: crosshair !important; }' : '';
    if (!on) closeComposer();
  }
  toggleBtn.addEventListener('click', function () {
    setMode(state.mode === 'comment' ? 'browse' : 'comment');
  });
  setMode('browse');

  // Figma-style shortcuts: C toggles Comment/Browse, Esc exits to Browse
  // (closing the composer first if one is open). Suppressed while typing.
  document.addEventListener('keydown', function (e) {
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    var t = e.target;
    if (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName))) return;
    if (e.composedPath && e.composedPath().some(function (n) { return n && n.tagName && /^(INPUT|TEXTAREA|SELECT)$/.test(n.tagName); })) return;
    if (e.key === 'c' || e.key === 'C') {
      setMode(state.mode === 'comment' ? 'browse' : 'comment');
      e.preventDefault();
    } else if (e.key === 'Escape') {
      if (composerEl) {
        closeComposer();
      } else if (popoverEl) {
        closeThread();
      } else if (state.mode === 'comment') {
        setMode('browse');
      } else {
        return;
      }
      e.preventDefault();
    }
  });

  // ---------- API ----------
  function api(path, opts) {
    opts = opts || {};
    opts.credentials = opts.credentials || 'include';
    opts.headers = opts.headers || {};
    opts.headers['X-Protopen-Client'] = 'runtime';
    if (opts.body && !opts.headers['Content-Type']) opts.headers['Content-Type'] = 'application/json';
    return fetch(apiBase + path, opts).then(function (r) {
      if (!r.ok) return r.text().then(function (t) { throw new Error(t || r.statusText); });
      return r.json();
    });
  }

  function bootstrap() {
    api('/api/sites/by-slug/' + orgSlug + '/' + siteSlug + '/comment-context').then(function (ctx) {
      state.siteId = ctx.siteId;
      state.deployId = ctx.deployId;
      state.isPublic = ctx.isPublic;
      state.siteName = ctx.siteName;
      loadComments().then(focusHashThread);
      loadDeploys();
    }).catch(function (err) {
      console.warn('protopen: bootstrap failed', err);
    });
    api('/api/session').then(function (s) { if (s && s.user) state.user = s.user; }).catch(function () {});
  }

  // Open the popover for the comment named in the URL hash, if any. Called
  // once after the first comment load and again on hashchange. Silently
  // no-ops if the comment isn't on this page (no pin rendered).
  function focusHashThread() {
    if (!state.activeThreadId) return;
    openThread(state.activeThreadId, null);
  }

  // Pull deploys list and render the version selector. Silently no-ops for
  // signed-out visitors (the endpoint requires org membership).
  function loadDeploys() {
    if (!state.siteId) return;
    api('/api/sites/' + state.siteId + '/deploys').then(function (resp) {
      var deploys = (resp && resp.deploys) || [];
      if (deploys.length < 2) return;
      renderVersionSelector(deploys);
    }).catch(function () { /* not a member — hide selector */ });
  }

  function siteAssetPath() {
    var prefix = '/~' + orgSlug + '/' + siteSlug;
    var p = location.pathname;
    if (p.indexOf(prefix) !== 0) return '/';
    p = p.slice(prefix.length) || '/';
    var m = p.match(/^\/_v\/[^/]+(\/.*)?$/);
    if (m) p = m[1] || '/';
    return p;
  }

  function deployUrl(deploy) {
    var prefix = '/~' + orgSlug + '/' + siteSlug;
    var asset = siteAssetPath();
    if (deploy.isCurrent) return prefix + asset;
    return prefix + '/_v/' + deploy.id + asset;
  }

  function renderVersionSelector(deploys) {
    var sel = document.createElement('select');
    sel.className = 'versions';
    sel.title = 'Switch deploy version';
    // listDeploys returns newest first; reverse so v1 is the first deploy.
    var ordered = deploys.slice().reverse();
    var activeIdx = -1;
    ordered.forEach(function (d, i) {
      var opt = document.createElement('option');
      opt.value = d.id;
      var label = 'v' + (i + 1);
      if (d.isCurrent) label += ' (Latest)';
      if (d.label) label += ' — ' + d.label;
      opt.textContent = label;
      if (d.id === (servedDeployId || state.deployId)) activeIdx = i;
      sel.appendChild(opt);
    });
    if (activeIdx >= 0) sel.selectedIndex = activeIdx;
    sel.addEventListener('change', function () {
      var picked = ordered[sel.selectedIndex];
      if (picked) window.location.href = deployUrl(picked);
    });
    topbarRight.insertBefore(sel, topbarRight.firstChild);
  }

  function loadComments() {
    if (!state.siteId) return Promise.resolve();
    // Skip pagePath filter when deep-linking to a specific comment. The
    // notification carries the comment's pagePath, so the iframe URL is
    // already right; but legacy data with mismatched pagePath would
    // otherwise be filtered out and the popover would never open.
    var qs = 'status=open';
    if (!state.activeThreadId) qs += '&pagePath=' + encodeURIComponent(location.pathname);
    return api('/api/sites/' + state.siteId + '/comments?' + qs).then(function (resp) {
      state.comments = resp.comments || [];
      renderPins();
    }).catch(function () {});
  }

  // ---------- selector capture + resolve ----------
  function captureSelector(el) {
    if (!el || el === document.body || el === document.documentElement) return null;
    var dataAttrs = ['data-comment-anchor', 'data-testid', 'data-id', 'data-component'];
    var cur = el;
    for (var hop = 0; hop < 4 && cur; hop++) {
      for (var i = 0; i < dataAttrs.length; i++) {
        var v = cur.getAttribute && cur.getAttribute(dataAttrs[i]);
        if (v) {
          return '[' + dataAttrs[i] + '="' + cssEscape(v) + '"]';
        }
      }
      cur = cur.parentElement;
    }
    if (el.id && /^[A-Za-z][A-Za-z0-9_-]*$/.test(el.id) && !/[0-9a-f]{6,}/i.test(el.id)) {
      return '#' + el.id;
    }
    var path = [];
    cur = el;
    for (var step = 0; step < 5 && cur && cur !== document.body; step++) {
      var part = cur.tagName.toLowerCase();
      if (cur.parentElement) {
        var siblings = Array.prototype.filter.call(cur.parentElement.children, function (c) { return c.tagName === cur.tagName; });
        if (siblings.length > 1) {
          var idx = siblings.indexOf(cur) + 1;
          part += ':nth-of-type(' + idx + ')';
        }
      }
      path.unshift(part);
      cur = cur.parentElement;
    }
    var sel = path.join(' > ');
    if (sel.length > 200) return null;
    return sel || null;
  }

  function cssEscape(s) {
    return String(s).replace(/["\\]/g, '\\$&');
  }

  function resolveSelector(sel) {
    if (!sel) return null;
    try {
      var matches = document.querySelectorAll(sel);
      if (matches.length === 1) return matches[0];
    } catch (e) {}
    return null;
  }

  // ---------- pin rendering ----------
  function pinPosition(c) {
    var el = resolveSelector(c.elementSelector);
    if (el && c.elementOffsetX != null && c.elementOffsetY != null) {
      var rect = el.getBoundingClientRect();
      return {
        x: rect.left + window.scrollX + rect.width * c.elementOffsetX,
        y: rect.top + window.scrollY + rect.height * c.elementOffsetY,
        anchored: true,
      };
    }
    if (c.pinX != null && c.pinY != null) {
      return {
        x: c.pinX * document.documentElement.scrollWidth,
        y: c.pinY * document.documentElement.scrollHeight,
        anchored: false,
      };
    }
    return null;
  }

  function rootComments() {
    return state.comments.filter(function (c) { return !c.parentId; });
  }

  function renderPins() {
    pinLayer.innerHTML = '';
    pinLayer.style.position = 'absolute';
    pinLayer.style.width = document.documentElement.scrollWidth + 'px';
    pinLayer.style.height = document.documentElement.scrollHeight + 'px';
    // Position pin layer in viewport coords (we'll place pins in page coords
    // and translate via window.scroll{X,Y}). Use a transform to compensate.
    pinLayer.style.transform = 'translate(' + (-window.scrollX) + 'px, ' + (-window.scrollY) + 'px)';

    var roots = rootComments();
    roots.forEach(function (c, idx) {
      var pos = pinPosition(c);
      if (!pos) return;
      var el = document.createElement('div');
      el.className = 'pin' + (pos.anchored ? '' : ' unanchored') + (c.id === state.activeThreadId ? ' active' : '');
      el.dataset.commentId = c.id;
      el.style.left = pos.x + 'px';
      el.style.top = pos.y + 'px';
      var inner = document.createElement('span');
      inner.textContent = String(idx + 1);
      el.appendChild(inner);
      attachDrag(el, c);
      pinLayer.appendChild(el);
    });
    countBadge.textContent = roots.length ? String(roots.length) : '';
  }

  function canMutate(c) {
    if (!state.user) return false;
    if (c.author && c.author.id === state.user.id) return true;
    var orgs = state.user.orgs || [];
    for (var i = 0; i < orgs.length; i++) {
      if (orgs[i].role === 'admin') return true;
    }
    return false;
  }

  var DRAG_THRESHOLD = 4;
  function attachDrag(pinEl, c) {
    var canDrag = canMutate(c);
    if (canDrag) pinEl.classList.add('draggable');
    pinEl.addEventListener('pointerdown', function (e) {
      if (e.button !== undefined && e.button !== 0) return;
      var startClientX = e.clientX;
      var startClientY = e.clientY;
      var startLeft = parseFloat(pinEl.style.left) || 0;
      var startTop = parseFloat(pinEl.style.top) || 0;
      var dragging = false;
      e.preventDefault();
      e.stopPropagation();
      try { pinEl.setPointerCapture(e.pointerId); } catch (err) {}

      function onMove(ev) {
        var dx = ev.clientX - startClientX;
        var dy = ev.clientY - startClientY;
        if (!dragging && (dx * dx + dy * dy) >= DRAG_THRESHOLD * DRAG_THRESHOLD) {
          if (!canDrag) return;
          dragging = true;
          closeThread();
          pinEl.classList.add('dragging');
        }
        if (dragging) {
          pinEl.style.left = (startLeft + dx) + 'px';
          pinEl.style.top = (startTop + dy) + 'px';
        }
      }
      function onUp(ev) {
        document.removeEventListener('pointermove', onMove, true);
        document.removeEventListener('pointerup', onUp, true);
        try { pinEl.releasePointerCapture(e.pointerId); } catch (err) {}
        if (!dragging) {
          openThread(c.id, pinEl);
          return;
        }
        pinEl.classList.remove('dragging');
        var newLeft = startLeft + (ev.clientX - startClientX);
        var newTop = startTop + (ev.clientY - startClientY);
        var w = Math.max(document.documentElement.scrollWidth, 1);
        var h = Math.max(document.documentElement.scrollHeight, 1);
        api('/api/comments/' + c.id, {
          method: 'PATCH',
          body: JSON.stringify({
            pinX: Math.max(0, Math.min(1, newLeft / w)),
            pinY: Math.max(0, Math.min(1, newTop / h)),
            clearAnchor: true,
          }),
        }).then(loadComments).catch(function (err) {
          alert('Failed to move pin: ' + err.message);
          loadComments();
        });
      }
      document.addEventListener('pointermove', onMove, true);
      document.addEventListener('pointerup', onUp, true);
    });
  }

  // ---------- thread popover ----------
  var popoverEl = null;
  function repliesFor(rootId) {
    return state.comments.filter(function (c) { return c.parentId === rootId; });
  }
  function initial(name) {
    return (name || '?').charAt(0).toUpperCase();
  }
  function relTime(iso) {
    var t = Date.parse(iso); if (isNaN(t)) return '';
    var delta = (Date.now() - t) / 1000;
    if (delta < 60) return 'just now';
    if (delta < 3600) return Math.floor(delta / 60) + 'm ago';
    if (delta < 86400) return Math.floor(delta / 3600) + 'h ago';
    return Math.floor(delta / 86400) + 'd ago';
  }
  var SEND_SVG = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12l14-7-7 14-2-5-5-2z"/></svg>';
  var CHECK_SVG = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6L9 17l-5-5"/></svg>';
  var DOTS_SVG = '<svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><circle cx="5" cy="12" r="1.5"/><circle cx="12" cy="12" r="1.5"/><circle cx="19" cy="12" r="1.5"/></svg>';
  var CLOSE_SVG = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6L6 18M6 6l12 12"/></svg>';

  // Set by buildToolbar when the kebab menu opens; cleared on close or when
  // the popover is dismissed. Keeps the outside-click handler from leaking
  // past a popover close (Esc with menu open, click outside the popover).
  var menuCleanup = null;

  function buildToolbar(c) {
    var bar = document.createElement('div');
    bar.className = 'pop-toolbar';
    if (state.user) {
      var resolveBtn = document.createElement('button');
      resolveBtn.type = 'button';
      resolveBtn.className = c.resolvedAt ? 'on' : '';
      resolveBtn.title = c.resolvedAt ? 'Mark unresolved' : 'Resolve thread';
      resolveBtn.innerHTML = CHECK_SVG;
      resolveBtn.addEventListener('click', function (e) {
        e.stopPropagation();
        api('/api/comments/' + c.id + '/resolve', { method: 'POST' })
          .then(function () { closeThread({ skipRender: true }); loadComments(); })
          .catch(function (err) { alert('Failed: ' + err.message); });
      });
      bar.appendChild(resolveBtn);
    }
    var menuBtn = document.createElement('button');
    menuBtn.type = 'button';
    menuBtn.title = 'More actions';
    menuBtn.innerHTML = DOTS_SVG;
    var menuEl = null;
    function closeMenu() {
      if (menuEl) menuEl.remove();
      menuEl = null;
      if (menuCleanup) { menuCleanup(); menuCleanup = null; }
    }
    function toggleMenu(e) {
      e.stopPropagation();
      if (menuEl) { closeMenu(); return; }
      menuEl = document.createElement('div');
      menuEl.className = 'pop-menu';
      var copyBtn = document.createElement('button');
      copyBtn.type = 'button';
      copyBtn.textContent = 'Copy link';
      copyBtn.addEventListener('click', function (ev) {
        ev.stopPropagation();
        var url = location.origin + location.pathname + '#protopen-comment=' + c.id;
        var done = function () {
          copyBtn.textContent = 'Copied';
          copyBtn.classList.add('copied');
          setTimeout(closeMenu, 800);
        };
        if (navigator.clipboard && navigator.clipboard.writeText) {
          navigator.clipboard.writeText(url).then(done).catch(done);
        } else {
          done();
        }
      });
      menuEl.appendChild(copyBtn);
      if (canMutate(c)) {
        var delBtn = document.createElement('button');
        delBtn.type = 'button';
        delBtn.textContent = 'Delete thread';
        delBtn.addEventListener('click', function (ev) {
          ev.stopPropagation();
          if (!confirm('Delete this comment thread? Replies will be removed too.')) return;
          api('/api/comments/' + c.id, { method: 'DELETE' })
            .then(function () { closeThread({ skipRender: true }); loadComments(); })
            .catch(function (err) { alert('Failed: ' + err.message); });
        });
        menuEl.appendChild(delBtn);
      }
      bar.appendChild(menuEl);
      setTimeout(function () {
        function onOutside(ev) {
          if (menuEl && !menuEl.contains(ev.target) && ev.target !== menuBtn) closeMenu();
        }
        document.addEventListener('click', onOutside, true);
        menuCleanup = function () { document.removeEventListener('click', onOutside, true); };
      }, 0);
    }
    menuBtn.addEventListener('click', toggleMenu);
    bar.appendChild(menuBtn);
    return bar;
  }

  function messageNode(c, isReply, trailing) {
    var msg = document.createElement('div');
    msg.className = isReply ? 'reply-msg' : 'msg';
    var head = document.createElement('div');
    head.className = 'msg-head';
    var av = document.createElement('div');
    av.className = 'avatar';
    var name = c.author ? c.author.name : (c.guestName || 'Guest');
    av.textContent = initial(name);
    var who = document.createElement('span');
    who.className = 'msg-author';
    who.textContent = name;
    var when = document.createElement('span');
    when.className = 'msg-time';
    when.textContent = relTime(c.createdAt);
    head.appendChild(av);
    head.appendChild(who);
    head.appendChild(when);
    if (trailing) head.appendChild(trailing);
    msg.appendChild(head);
    var body = document.createElement('div');
    body.className = 'msg-body';
    body.textContent = c.body;
    msg.appendChild(body);
    return msg;
  }
  function openThread(rootId, pinEl) {
    closeThread();
    var found = state.comments.find(function (c) { return c.id === rootId; });
    if (!found) return;
    // Deep-links from inbox notifications can point at a reply, not the root.
    // Walk up to the root so the popover anchors to the thread's pin.
    if (found.parentId) {
      var parent = state.comments.find(function (c) { return c.id === found.parentId; });
      if (parent) {
        rootId = parent.id;
        found = parent;
      }
    }
    var root = found;
    if (!pinEl) pinEl = pinLayer.querySelector('[data-comment-id="' + rootId + '"]');
    state.activeThreadId = rootId;

    popoverEl = document.createElement('div');
    popoverEl.className = 'popover';
    // Hide until positioned to avoid a one-frame flash at the shadow root's
    // top-left corner before positionPopover runs.
    popoverEl.style.visibility = 'hidden';

    var body = document.createElement('div');
    body.className = 'pop-body';
    body.appendChild(messageNode(root, false, buildToolbar(root)));
    var replies = repliesFor(rootId);
    if (replies.length) {
      var rWrap = document.createElement('div');
      rWrap.className = 'replies';
      replies.forEach(function (r) { rWrap.appendChild(messageNode(r, true)); });
      body.appendChild(rWrap);
    }
    popoverEl.appendChild(body);

    if (state.user) {
      var compose = document.createElement('div');
      compose.className = 'pop-compose';
      var ta = document.createElement('textarea');
      ta.rows = 1;
      ta.placeholder = 'Reply…';
      var send = document.createElement('button');
      send.className = 'send';
      send.type = 'button';
      send.title = 'Reply';
      function submitReply() {
        var text = ta.value.trim();
        if (!text) return;
        postComment({ parentId: rootId, body: text, pagePath: location.pathname })
          .then(function () { loadComments().then(function () { openThread(rootId, null); }); })
          .catch(function (e) { alert('Failed: ' + e.message); });
      }
      attachSendButton(ta, send, submitReply);
      attachMentions(ta);
      attachAutoGrow(ta, 80);
      attachEnterToSubmit(ta, submitReply);
      compose.appendChild(ta);
      compose.appendChild(send);
      popoverEl.appendChild(compose);
      // Defer focus a tick so the popover finishes painting before the
      // textarea grabs focus (otherwise iOS skips the keyboard).
      setTimeout(function () { ta.focus(); }, 0);
    }

    shadow.appendChild(popoverEl);
    positionPopover(popoverEl, pinEl);
    popoverEl.style.visibility = '';
    renderPins();
  }
  function closeThread(opts) {
    if (menuCleanup) { menuCleanup(); menuCleanup = null; }
    if (popoverEl && popoverEl.parentNode) popoverEl.parentNode.removeChild(popoverEl);
    popoverEl = null;
    if (state.activeThreadId) {
      state.activeThreadId = null;
      if (!opts || !opts.skipRender) renderPins();
    }
  }
  function positionPopover(pop, pinEl) {
    var popW = pop.offsetWidth || 320;
    var popH = pop.offsetHeight || 200;
    var left, top;
    if (pinEl) {
      var pinRect = pinEl.getBoundingClientRect();
      left = pinRect.right + 12 + window.scrollX;
      if (left + popW > window.scrollX + window.innerWidth - 8) {
        left = pinRect.left - popW - 12 + window.scrollX;
      }
      if (left < window.scrollX + 8) left = window.scrollX + 8;
      top = pinRect.top + window.scrollY;
    } else {
      // Pinless comment (legacy / unanchored deep-link): anchor below the
      // topbar on the right edge instead of next to a pin.
      left = window.scrollX + window.innerWidth - popW - 16;
      top = window.scrollY + TOPBAR_H + 12;
    }
    var minTop = window.scrollY + TOPBAR_H + 4;
    var maxTop = window.scrollY + window.innerHeight - popH - 8;
    if (top < minTop) top = minTop;
    if (top > maxTop) top = Math.max(minTop, maxTop);
    pop.style.left = left + 'px';
    pop.style.top = top + 'px';
  }

  // Click outside the popover or any pin closes the thread.
  document.addEventListener('click', function (e) {
    if (!popoverEl) return;
    var path = e.composedPath ? e.composedPath() : [];
    for (var i = 0; i < path.length; i++) {
      var n = path[i];
      if (n === popoverEl) return;
      if (n && n.classList && n.classList.contains('pin')) return;
    }
    closeThread();
  }, true);

  // ---------- composer ----------
  var composerEl = null;
  function openComposer(pin) {
    closeComposer();
    composerEl = document.createElement('div');
    composerEl.className = 'composer';
    var maxLeft = window.innerWidth - 380;
    var maxTop = window.innerHeight - 160;
    composerEl.style.left = Math.max(8, Math.min(maxLeft, pin.viewportX + 16)) + 'px';
    composerEl.style.top = Math.max(8, Math.min(maxTop, pin.viewportY + 16)) + 'px';

    if (!state.user) {
      var signInURL = apiBase + '/';
      composerEl.innerHTML =
        '<div class="signin">Sign in to leave a comment. <a href="' + escapeHTML(signInURL) + '" target="_blank" rel="noopener">Open sign-in →</a></div>' +
        '<button class="close" type="button" aria-label="Close">' + CLOSE_SVG + '</button>';
      shadow.appendChild(composerEl);
      composerEl.querySelector('.close').addEventListener('click', closeComposer);
      return;
    }

    composerEl.innerHTML =
      '<textarea class="body-input" rows="1" placeholder="Add a comment… @ to mention"></textarea>' +
      '<button class="send" type="button" title="Comment"></button>';
    shadow.appendChild(composerEl);
    var ta = composerEl.querySelector('.body-input');
    var sendBtn = composerEl.querySelector('.send');
    ta.focus();
    attachMentions(ta);
    attachAutoGrow(ta, 120);
    function submitNew() {
      var text = ta.value.trim();
      if (!text) return;
      postComment({
        body: text,
        pagePath: location.pathname,
        pinX: pin.pinX,
        pinY: pin.pinY,
        elementSelector: pin.selector || undefined,
        elementOffsetX: pin.offsetX,
        elementOffsetY: pin.offsetY,
      }).then(function () {
        closeComposer();
        loadComments();
      }).catch(function (e) {
        alert('Failed: ' + e.message);
      });
    }
    attachSendButton(ta, sendBtn, submitNew);
    attachEnterToSubmit(ta, submitNew);
  }

  function closeComposer() {
    if (composerEl && composerEl.parentNode) composerEl.parentNode.removeChild(composerEl);
    composerEl = null;
  }

  function postComment(payload) {
    return api('/api/sites/' + state.siteId + '/comments', {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  }

  function escapeHTML(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c];
    });
  }

  // ---------- click capture (Comment mode) ----------
  document.addEventListener('click', function (e) {
    if (state.mode !== 'comment') return;
    if (e.composedPath && e.composedPath().indexOf(host) !== -1) return;
    if (e.target.closest && e.target.closest('a[href]')) return;
    e.preventDefault();
    e.stopPropagation();
    var target = e.target;
    var rect = target.getBoundingClientRect();
    var selector = captureSelector(target);
    var offsetX = rect.width > 0 ? (e.clientX - rect.left) / rect.width : 0.5;
    var offsetY = rect.height > 0 ? (e.clientY - rect.top) / rect.height : 0.5;
    var pin = {
      selector: selector,
      offsetX: offsetX,
      offsetY: offsetY,
      pinX: e.pageX / Math.max(document.documentElement.scrollWidth, 1),
      pinY: e.pageY / Math.max(document.documentElement.scrollHeight, 1),
      viewportX: e.clientX,
      viewportY: e.clientY,
    };
    openComposer(pin);
  }, true);

  // ---------- textarea ergonomics ----------
  // attachSendButton wires the textarea + send-button pair used by both the
  // popover reply row and the new-comment composer: the button stays gray
  // until the textarea has non-empty content, then lights up orange.
  function attachSendButton(textarea, btn, submit) {
    btn.innerHTML = SEND_SVG;
    function sync() { btn.classList.toggle('active', textarea.value.trim().length > 0); }
    textarea.addEventListener('input', sync);
    btn.addEventListener('click', submit);
    sync();
  }

  function attachAutoGrow(textarea, maxH) {
    function grow() {
      textarea.style.height = 'auto';
      textarea.style.height = Math.min(textarea.scrollHeight, maxH) + 'px';
    }
    textarea.addEventListener('input', grow);
    grow();
  }
  function attachEnterToSubmit(textarea, submit) {
    textarea.addEventListener('keydown', function (e) {
      if (e.key !== 'Enter' || e.shiftKey || e.isComposing) return;
      e.preventDefault();
      submit();
    });
  }

  // ---------- mention autocomplete (signed-in only) ----------
  function attachMentions(textarea) {
    var dropdown = null;
    var selectedIdx = 0;
    var matches = [];
    var atPos = -1;
    var fetchTimer = null;

    function close() {
      if (dropdown && dropdown.parentNode) dropdown.parentNode.removeChild(dropdown);
      dropdown = null;
      matches = [];
      atPos = -1;
    }

    function render() {
      if (!matches.length) { close(); return; }
      if (!dropdown) {
        dropdown = document.createElement('div');
        dropdown.className = 'mentions';
        shadow.appendChild(dropdown);
      }
      var rect = textarea.getBoundingClientRect();
      dropdown.style.position = 'fixed';
      dropdown.style.left = rect.left + 'px';
      dropdown.style.top = (rect.bottom + 4) + 'px';
      dropdown.innerHTML = '';
      matches.forEach(function (m, i) {
        var d = document.createElement('div');
        d.textContent = '@' + m.username + ' — ' + m.name;
        if (i === selectedIdx) d.className = 'selected';
        d.addEventListener('mousedown', function (e) { e.preventDefault(); pick(i); });
        dropdown.appendChild(d);
      });
    }

    function pick(i) {
      var m = matches[i];
      if (!m) return;
      var val = textarea.value;
      var before = val.slice(0, atPos);
      var after = val.slice(textarea.selectionStart);
      textarea.value = before + '@' + m.username + ' ' + after;
      var newPos = before.length + m.username.length + 2;
      textarea.setSelectionRange(newPos, newPos);
      close();
    }

    textarea.addEventListener('keyup', function (e) {
      if (!state.user || !state.siteId) return;
      if (e.key === 'Escape') { close(); return; }
      var pos = textarea.selectionStart;
      var val = textarea.value;
      // Find a @ before the cursor with no whitespace in between.
      var i = pos - 1, foundAt = -1;
      while (i >= 0 && /[A-Za-z0-9_-]/.test(val[i])) i--;
      if (i >= 0 && val[i] === '@' && (i === 0 || /\s/.test(val[i - 1]))) foundAt = i;
      if (foundAt === -1) { close(); return; }
      atPos = foundAt;
      var q = val.slice(foundAt + 1, pos);
      if (fetchTimer) clearTimeout(fetchTimer);
      fetchTimer = setTimeout(function () {
        api('/api/sites/' + state.siteId + '/mention-candidates?q=' + encodeURIComponent(q)).then(function (resp) {
          matches = resp.candidates || [];
          selectedIdx = 0;
          render();
        }).catch(close);
      }, 150);
    });

    textarea.addEventListener('keydown', function (e) {
      if (!dropdown || !matches.length) return;
      // stopImmediatePropagation prevents the submit-on-Enter handler from
      // also firing on the same event — mentions runs first because it's
      // attached first at each call site.
      if (e.key === 'ArrowDown') { selectedIdx = (selectedIdx + 1) % matches.length; render(); e.preventDefault(); e.stopImmediatePropagation(); }
      else if (e.key === 'ArrowUp') { selectedIdx = (selectedIdx - 1 + matches.length) % matches.length; render(); e.preventDefault(); e.stopImmediatePropagation(); }
      else if (e.key === 'Enter') { pick(selectedIdx); e.preventDefault(); e.stopImmediatePropagation(); }
      else if (e.key === 'Escape') { close(); e.preventDefault(); e.stopImmediatePropagation(); }
    });

    textarea.addEventListener('blur', function () { setTimeout(close, 100); });
  }

  // ---------- recompute pins on layout changes ----------
  var rerenderScheduled = false;
  function scheduleRerender() {
    if (rerenderScheduled) return;
    rerenderScheduled = true;
    requestAnimationFrame(function () { rerenderScheduled = false; renderPins(); });
  }
  window.addEventListener('scroll', scheduleRerender, { passive: true });
  window.addEventListener('resize', scheduleRerender);
  try {
    var ro = new ResizeObserver(scheduleRerender);
    ro.observe(document.documentElement);
    ro.observe(document.body);
    new MutationObserver(function () { setTimeout(scheduleRerender, 50); })
      .observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ['style', 'class'] });
  } catch (e) {}

  // SPA navigation: reload comments when the path changes.
  var lastPath = location.pathname;
  function maybeNavigate() {
    if (location.pathname !== lastPath) {
      lastPath = location.pathname;
      state.activeThreadId = null;
      loadComments();
    }
  }
  ['pushState', 'replaceState'].forEach(function (m) {
    var orig = history[m];
    history[m] = function () { var r = orig.apply(this, arguments); maybeNavigate(); return r; };
  });
  window.addEventListener('popstate', maybeNavigate);

  // Deep-link: highlight a specific comment by hash #protopen-comment=<id>.
  function maybeFocusFromHash() {
    var m = location.hash.match(/protopen-comment=([a-zA-Z0-9_]+)/);
    if (m) state.activeThreadId = m[1];
  }
  maybeFocusFromHash();
  window.addEventListener('hashchange', function () { maybeFocusFromHash(); renderPins(); focusHashThread(); });

  bootstrap();
})();

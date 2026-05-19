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
  if (!orgSlug || !siteSlug) return;

  function deriveSlugFromPath(index) {
    var parts = location.pathname.split('/').filter(Boolean);
    if (!parts.length || parts[0].charAt(0) !== '~') return '';
    if (index === 0) return parts[0].slice(1);
    return parts[1] || '';
  }

  // ---------- state ----------
  var state = {
    mode: 'browse',          // 'browse' | 'comment'
    comments: [],            // server-side list, scoped to site
    siteId: null,
    deployId: null,
    isPublic: false,
    siteName: '',
    user: null,              // signed-in session user, if any
    guestName: (function () { try { return localStorage.getItem('protopen.guestName') || ''; } catch (e) { return ''; } })(),
    pendingPin: null,        // captured but not yet posted
    activeThreadId: null,    // currently-open thread in sidebar
    mentionCandidates: [],
  };

  // ---------- shadow root + chrome ----------
  var host = document.createElement('div');
  host.id = '__protopen_host';
  host.style.cssText = 'position:fixed;inset:0;pointer-events:none;z-index:2147483646';
  document.documentElement.appendChild(host);
  var shadow = host.attachShadow({ mode: 'open' });

  shadow.innerHTML = (
    '<style>' +
    ':host, * { box-sizing: border-box; font-family: system-ui, -apple-system, "Segoe UI", sans-serif; }' +
    '.pin-layer { position: absolute; inset: 0; pointer-events: none; }' +
    '.pin { position: absolute; width: 28px; height: 28px; border-radius: 50% 50% 50% 0; background: #ff8f52; color: white; font: 600 12px/28px system-ui, sans-serif; text-align: center; transform: translate(-14px, -28px) rotate(-45deg); box-shadow: 0 2px 8px rgba(0,0,0,.3); pointer-events: auto; cursor: pointer; transition: transform .15s; }' +
    '.pin:hover { transform: translate(-14px, -28px) rotate(-45deg) scale(1.1); }' +
    '.pin span { display: block; transform: rotate(45deg); }' +
    '.pin.unanchored { border: 2px dashed rgba(255,255,255,.6); }' +
    '.pin.active { background: #0066ff; }' +
    '.topbar { position: fixed; top: 0; left: 0; right: 0; height: 40px; background: #111; color: white; display: flex; align-items: center; justify-content: space-between; padding: 0 16px; font: 500 13px system-ui; box-shadow: 0 1px 4px rgba(0,0,0,.25); pointer-events: auto; z-index: 1; }' +
    '.topbar .brand { display: flex; align-items: center; gap: 8px; opacity: .7; font-size: 12px; letter-spacing: .02em; text-transform: uppercase; }' +
    '.topbar .brand::before { content: ""; display: inline-block; width: 10px; height: 10px; border-radius: 50% 50% 50% 0; background: #ff8f52; transform: rotate(-45deg); }' +
    '.topbar .toggle-btn { display: inline-flex; align-items: center; gap: 8px; background: transparent; color: inherit; border: 1px solid rgba(255,255,255,.2); padding: 6px 14px; border-radius: 999px; cursor: pointer; font: 500 13px system-ui; transition: background .15s, border-color .15s; }' +
    '.topbar .toggle-btn:hover { border-color: rgba(255,255,255,.4); }' +
    '.topbar .toggle-btn.active { background: #ff8f52; border-color: #ff8f52; }' +
    '.topbar .toggle-btn .count { display: inline-flex; min-width: 18px; height: 18px; padding: 0 6px; align-items: center; justify-content: center; background: rgba(255,255,255,.22); border-radius: 999px; font-size: 11px; }' +
    '.topbar .toggle-btn .count:empty { display: none; }' +
    '.topbar .shortcut { opacity: .55; font-size: 11px; margin-left: 4px; }' +
    '.sidebar { position: fixed; top: 0; right: 0; width: 360px; max-width: 100vw; height: 100vh; background: white; color: #111; border-left: 1px solid #e5e5e5; box-shadow: -4px 0 24px rgba(0,0,0,.15); pointer-events: auto; transform: translateX(100%); transition: transform .2s ease-out; display: flex; flex-direction: column; }' +
    '.sidebar.open { transform: translateX(0); }' +
    '.sidebar header { padding: 16px; border-bottom: 1px solid #eee; display: flex; align-items: center; justify-content: space-between; }' +
    '.sidebar header strong { font-size: 14px; }' +
    '.sidebar header button { background: none; border: 0; cursor: pointer; font-size: 18px; color: #666; padding: 4px 8px; }' +
    '.threads { flex: 1; overflow-y: auto; padding: 8px; }' +
    '.thread { border: 1px solid #eee; border-radius: 8px; padding: 12px; margin-bottom: 8px; cursor: pointer; }' +
    '.thread.active { border-color: #0066ff; }' +
    '.thread .meta { font-size: 12px; color: #888; margin-bottom: 4px; }' +
    '.thread .body { font-size: 13px; line-height: 1.4; white-space: pre-wrap; word-wrap: break-word; }' +
    '.thread .reply { font-size: 12px; color: #444; margin-top: 8px; padding-left: 8px; border-left: 2px solid #eee; }' +
    '.reply-input { display: none; margin-top: 8px; }' +
    '.thread.active .reply-input { display: block; }' +
    '.reply-input textarea { width: 100%; min-height: 60px; padding: 8px; border: 1px solid #ddd; border-radius: 6px; font: 13px system-ui; resize: vertical; }' +
    '.reply-input .actions { display: flex; gap: 8px; justify-content: flex-end; margin-top: 6px; }' +
    '.composer { position: fixed; background: white; color: #111; border: 1px solid #ddd; border-radius: 12px; padding: 12px; box-shadow: 0 8px 32px rgba(0,0,0,.18); width: 320px; pointer-events: auto; z-index: 2; }' +
    '.composer label { display: block; font-size: 12px; color: #666; margin-bottom: 4px; }' +
    '.composer input, .composer textarea { width: 100%; padding: 8px; border: 1px solid #ddd; border-radius: 6px; font: 13px system-ui; }' +
    '.composer textarea { min-height: 80px; margin-top: 8px; resize: vertical; }' +
    '.composer .actions { display: flex; gap: 8px; justify-content: flex-end; margin-top: 10px; }' +
    '.composer button { background: #ff8f52; color: white; border: 0; padding: 6px 14px; border-radius: 6px; cursor: pointer; font: 500 13px system-ui; }' +
    '.composer button.secondary { background: #eee; color: #333; }' +
    '.mentions { position: absolute; background: white; border: 1px solid #ddd; border-radius: 6px; box-shadow: 0 4px 12px rgba(0,0,0,.12); margin-top: 2px; max-height: 200px; overflow-y: auto; min-width: 200px; z-index: 3; }' +
    '.mentions div { padding: 6px 10px; cursor: pointer; font-size: 13px; }' +
    '.mentions div:hover, .mentions div.selected { background: #f0f7ff; }' +
    '.author { font-weight: 600; color: #222; }' +
    '.guest { color: #888; font-weight: 400; }' +
    '</style>' +
    '<div class="pin-layer"></div>' +
    '<div class="topbar">' +
      '<div class="brand">Protopen</div>' +
      '<button class="toggle-btn" aria-pressed="false">' +
        '<span class="label">Comments</span>' +
        '<span class="count"></span>' +
        '<span class="shortcut">C</span>' +
      '</button>' +
    '</div>' +
    '<aside class="sidebar">' +
      '<header><strong>Comments</strong><button class="sidebar-close" aria-label="Close">×</button></header>' +
      '<div class="threads"></div>' +
    '</aside>'
  );

  var pinLayer = shadow.querySelector('.pin-layer');
  var toggleBtn = shadow.querySelector('.toggle-btn');
  var countBadge = shadow.querySelector('.toggle-btn .count');
  var sidebar = shadow.querySelector('.sidebar');
  var sidebarClose = shadow.querySelector('.sidebar-close');
  var threadsEl = shadow.querySelector('.threads');

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
    if (on) sidebar.classList.add('open');
    else closeComposer();
  }
  toggleBtn.addEventListener('click', function () {
    setMode(state.mode === 'comment' ? 'browse' : 'comment');
  });
  sidebarClose.addEventListener('click', function () { sidebar.classList.remove('open'); });
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
      loadComments();
    }).catch(function (err) {
      console.warn('protopen: bootstrap failed', err);
    });
    api('/api/session').then(function (s) { if (s && s.user) state.user = s.user; }).catch(function () {});
  }

  function loadComments() {
    if (!state.siteId) return;
    var path = encodeURIComponent(location.pathname);
    api('/api/sites/' + state.siteId + '/comments?status=all&pagePath=' + path).then(function (resp) {
      state.comments = resp.comments || [];
      renderPins();
      renderSidebar();
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

  function repliesFor(id) {
    return state.comments.filter(function (c) { return c.parentId === id; });
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
      el.addEventListener('click', function (e) {
        e.stopPropagation();
        openThread(c.id);
      });
      pinLayer.appendChild(el);
    });
    countBadge.textContent = roots.length ? String(roots.length) : '';
  }

  // ---------- sidebar ----------
  function renderSidebar() {
    threadsEl.innerHTML = '';
    var roots = rootComments();
    if (!roots.length) {
      var empty = document.createElement('div');
      empty.style.cssText = 'padding:24px;color:#888;font-size:13px;text-align:center;';
      empty.textContent = state.mode === 'comment' ? 'Click anywhere on the page to leave a comment.' : 'No comments yet on this page.';
      threadsEl.appendChild(empty);
      return;
    }
    roots.forEach(function (c, idx) {
      var thread = document.createElement('div');
      thread.className = 'thread' + (c.id === state.activeThreadId ? ' active' : '');
      thread.dataset.commentId = c.id;
      thread.addEventListener('click', function () { openThread(c.id); });

      var meta = document.createElement('div');
      meta.className = 'meta';
      var authorLabel = c.author ? '<span class="author">' + escapeHTML(c.author.name) + '</span>' : '<span class="guest">' + escapeHTML(c.guestName || 'Guest') + '</span>';
      meta.innerHTML = '#' + (idx + 1) + ' &middot; ' + authorLabel + ' &middot; ' + relTime(c.createdAt);
      thread.appendChild(meta);

      var body = document.createElement('div');
      body.className = 'body';
      body.textContent = c.body;
      thread.appendChild(body);

      repliesFor(c.id).forEach(function (r) {
        var rEl = document.createElement('div');
        rEl.className = 'reply';
        var rAuthor = r.author ? r.author.name : (r.guestName || 'Guest');
        rEl.innerHTML = '<div class="meta">' + escapeHTML(rAuthor) + ' &middot; ' + relTime(r.createdAt) + '</div>' + '<div>' + escapeHTML(r.body) + '</div>';
        thread.appendChild(rEl);
      });

      var replyBox = document.createElement('div');
      replyBox.className = 'reply-input';
      var ta = document.createElement('textarea');
      ta.placeholder = 'Reply…';
      replyBox.appendChild(ta);
      attachMentions(ta);
      var actions = document.createElement('div');
      actions.className = 'actions';
      var send = document.createElement('button');
      send.textContent = 'Reply';
      send.addEventListener('click', function (e) {
        e.stopPropagation();
        var body = ta.value.trim();
        if (!body) return;
        postComment({ parentId: c.id, body: body, pagePath: location.pathname }).then(function () {
          ta.value = '';
          loadComments();
        });
      });
      actions.appendChild(send);
      replyBox.appendChild(actions);
      thread.appendChild(replyBox);

      threadsEl.appendChild(thread);
    });
  }

  function openThread(id) {
    state.activeThreadId = id;
    sidebar.classList.add('open');
    renderPins();
    renderSidebar();
    var active = threadsEl.querySelector('.thread.active');
    if (active && active.scrollIntoView) active.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
  }

  // ---------- composer ----------
  var composerEl = null;
  function openComposer(pin) {
    closeComposer();
    composerEl = document.createElement('div');
    composerEl.className = 'composer';
    // Position near the click, clamping to viewport
    var maxLeft = window.innerWidth - 340;
    var maxTop = window.innerHeight - 220;
    composerEl.style.left = Math.max(8, Math.min(maxLeft, pin.viewportX + 16)) + 'px';
    composerEl.style.top = Math.max(8, Math.min(maxTop, pin.viewportY + 16)) + 'px';
    var needsName = !state.user && !state.guestName;
    var html = '';
    if (needsName) {
      html += '<label>Your name</label><input class="name-input" maxlength="60" placeholder="Sam" />';
    }
    html += '<textarea class="body-input" placeholder="Add a comment… use @ to mention"></textarea>';
    html += '<div class="actions"><button class="secondary cancel">Cancel</button><button class="submit">Comment</button></div>';
    composerEl.innerHTML = html;
    shadow.appendChild(composerEl);
    var ta = composerEl.querySelector('.body-input');
    var nameInput = composerEl.querySelector('.name-input');
    if (nameInput) nameInput.focus(); else ta.focus();
    attachMentions(ta);
    composerEl.querySelector('.cancel').addEventListener('click', closeComposer);
    composerEl.querySelector('.submit').addEventListener('click', function () {
      var body = ta.value.trim();
      if (!body) return;
      var name = nameInput ? sanitizeName(nameInput.value) : '';
      if (!state.user && !name && !state.guestName) {
        nameInput.focus();
        return;
      }
      if (name) {
        state.guestName = name;
        try { localStorage.setItem('protopen.guestName', name); } catch (e) {}
      }
      postComment({
        body: body,
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
    });
  }

  function closeComposer() {
    if (composerEl && composerEl.parentNode) composerEl.parentNode.removeChild(composerEl);
    composerEl = null;
  }

  function postComment(payload) {
    if (!state.user) payload.guestName = state.guestName;
    return api('/api/sites/' + state.siteId + '/comments', {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  }

  function sanitizeName(s) {
    s = String(s || '').replace(/[\x00-\x1f]+/g, ' ').replace(/\s+/g, ' ').trim();
    if (s.length > 60) s = s.slice(0, 60);
    return s;
  }

  function escapeHTML(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c];
    });
  }

  function relTime(iso) {
    var t = Date.parse(iso); if (isNaN(t)) return '';
    var delta = (Date.now() - t) / 1000;
    if (delta < 60) return 'just now';
    if (delta < 3600) return Math.floor(delta / 60) + 'm ago';
    if (delta < 86400) return Math.floor(delta / 3600) + 'h ago';
    return Math.floor(delta / 86400) + 'd ago';
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
      if (e.key === 'ArrowDown') { selectedIdx = (selectedIdx + 1) % matches.length; render(); e.preventDefault(); }
      else if (e.key === 'ArrowUp') { selectedIdx = (selectedIdx - 1 + matches.length) % matches.length; render(); e.preventDefault(); }
      else if (e.key === 'Enter') { pick(selectedIdx); e.preventDefault(); }
      else if (e.key === 'Escape') { close(); e.preventDefault(); }
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

  // Deep-link: open a specific comment by hash #protopen-comment=<id>.
  function maybeFocusFromHash() {
    var m = location.hash.match(/protopen-comment=([a-zA-Z0-9_]+)/);
    if (m) {
      // Try once now and again after comments load.
      var id = m[1];
      state.activeThreadId = id;
      sidebar.classList.add('open');
    }
  }
  maybeFocusFromHash();
  window.addEventListener('hashchange', function () { maybeFocusFromHash(); renderPins(); renderSidebar(); });

  bootstrap();
})();

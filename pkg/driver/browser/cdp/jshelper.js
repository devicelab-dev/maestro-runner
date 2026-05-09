window.__maestro = {
  // Tracks how many cross-origin iframes _collectRoots skipped on the most
  // recent walk. Read by the Go side to surface a clear error when a query
  // misses (so users know the cause is OOPIF, not a typo'd selector).
  _lastCrossOriginSkips: 0,

  // Collect every same-origin DOM root reachable from the top frame:
  //   * the top document
  //   * same-origin iframe / frame contentDocuments (recursively)
  //   * any open shadowRoot (mode: "open") attached to any element in any
  //     of the above (recursively)
  //
  // Document and ShadowRoot share the relevant DOM API surface
  // (querySelectorAll / getElementById), so callers can treat the returned
  // list uniformly.
  //
  // Cross-origin iframes throw on contentDocument access; we swallow and
  // count them so the caller can warn. Closed shadow roots
  // (`mode: "closed"`) return null from `el.shadowRoot` and are not
  // detectable from outside — same constraint Maestro CLI has, no fix
  // possible without privileged WebDriver access.
  _collectRoots: function() {
    var roots = [document];
    var skipped = 0;
    function walk(root) {
      var elements;
      try { elements = root.querySelectorAll('*'); } catch (e) { return; }
      for (var i = 0; i < elements.length; i++) {
        var el = elements[i];
        var tag = el.tagName;
        if (tag === 'IFRAME' || tag === 'FRAME') {
          var inner = null;
          try { inner = el.contentDocument; } catch (e) { skipped++; continue; }
          if (inner === null) {
            // cross-origin OR sandboxed iframe (sandbox without
            // allow-same-origin) returns null
            skipped++;
            continue;
          }
          if (roots.indexOf(inner) === -1) {
            roots.push(inner);
            walk(inner);
          }
          continue;
        }
        // Open shadow root attached to this element — descend
        if (el.shadowRoot && roots.indexOf(el.shadowRoot) === -1) {
          roots.push(el.shadowRoot);
          walk(el.shadowRoot);
        }
      }
    }
    walk(document);
    this._lastCrossOriginSkips = skipped;
    return roots;
  },

  // Returns the number of cross-origin / sandboxed frames skipped during the
  // most recent _collectRoots pass. Used by the Go finders to enrich
  // not-found error messages.
  getLastCrossOriginSkips: function() {
    return this._lastCrossOriginSkips || 0;
  },

  findByText: function(text) {
    var lower = text.toLowerCase();
    var docs = this._collectRoots();
    var best = null, bestDepth = -1;
    for (var d = 0; d < docs.length; d++) {
      var all;
      try { all = docs[d].querySelectorAll('*'); } catch (e) { continue; }
      for (var i = 0; i < all.length; i++) {
        var el = all[i];
        var t = (el.textContent || '').trim().toLowerCase();
        var label = (el.getAttribute && (el.getAttribute('aria-label') || '')).toLowerCase();
        var ph = (el.getAttribute && (el.getAttribute('placeholder') || '')).toLowerCase();
        if (t.indexOf(lower) !== -1 || label.indexOf(lower) !== -1 || ph.indexOf(lower) !== -1) {
          var depth = 0, n = el;
          while (n.parentElement) { depth++; n = n.parentElement; }
          if (depth > bestDepth) { best = el; bestDepth = depth; }
        }
      }
    }
    if (!best) throw new Error('not found: ' + text);
    var p = best;
    var bestBody = (best.ownerDocument && best.ownerDocument.body) || document.body;
    while (p && p !== bestBody) {
      var tag = p.tagName.toLowerCase();
      if (['a','button','input','select','textarea'].indexOf(tag) !== -1 ||
          p.getAttribute('role') === 'button' || p.getAttribute('tabindex') !== null) return p;
      p = p.parentElement;
    }
    return best;
  },

  // Find first element matching a CSS selector across same-origin frames.
  // Used by Go finders as a fallback when the top-frame Rod lookup fails.
  findByCSSAcrossFrames: function(selector) {
    var docs = this._collectRoots();
    for (var d = 0; d < docs.length; d++) {
      try {
        var el = docs[d].querySelector(selector);
        if (el) return el;
      } catch (e) {}
    }
    throw new Error('not found: ' + selector);
  },

  // Two-stage cross-root text-index lookup. Step 1 enumerates all matches
  // across same-origin frames + open shadow roots, stashes the requested
  // element in _lastIndexedMatch, and returns the total match count by
  // value. Step 2 retrieves the stashed element as a remote handle.
  //
  // The split is structural, not optional: rod.Eval(...).ByObject() can
  // stall when the result is a JS null (no remote objectId to track), so
  // we never request .ByObject() unless we know an element exists.
  // (Issue #72.)
  findByTextAt_count: function(text, index) {
    // Strict, semantic matching for `text + index`. An element matches only
    // when ALL of:
    //  * its own text (direct text-node children, no descendants), or its
    //    aria-label, or placeholder equals `text` (case-insensitive,
    //    trimmed). Avoids matching ancestors whose `textContent` transitively
    //    includes the match.
    //  * it's the kind of element a user could meaningfully act on — an
    //    <a>/<button>/<input>/<select>/<textarea>, or has role/tabindex/
    //    aria-label attached. Filters out decorative/structural elements
    //    like <code>, <p>, <h1> that would otherwise inflate the index.
    // This approximates Maestro CLI's AX-tree-named-element behaviour
    // without the cost of a per-frame Accessibility.queryAXTree round-trip.
    // Issue #72.
    var lower = text.toLowerCase();
    var INTERACTIVE_TAGS = { A:1, BUTTON:1, INPUT:1, SELECT:1, TEXTAREA:1 };
    function isInteractive(el) {
      if (INTERACTIVE_TAGS[el.tagName]) return true;
      if (!el.getAttribute) return false;
      if (el.getAttribute('role')) return true;
      if (el.getAttribute('tabindex') !== null) return true;
      if (el.getAttribute('aria-label')) return true;
      return false;
    }
    var roots = this._collectRoots();
    var matches = [];
    for (var r = 0; r < roots.length; r++) {
      var all;
      try { all = roots[r].querySelectorAll('*'); } catch (e) { continue; }
      for (var i = 0; i < all.length; i++) {
        var el = all[i];
        if (!isInteractive(el)) continue;
        var label = (el.getAttribute('aria-label') || '').toLowerCase();
        var ph = (el.getAttribute('placeholder') || '').toLowerCase();
        var ownText = '';
        for (var c = 0; c < el.childNodes.length; c++) {
          var n = el.childNodes[c];
          if (n.nodeType === 3) ownText += n.nodeValue;
        }
        ownText = ownText.trim().toLowerCase();
        if (ownText === lower || label === lower || ph === lower) {
          matches.push(el);
        }
      }
    }
    this._lastIndexedMatch = matches[index] || null;
    return matches.length;
  },

  findByTextAt_get: function() {
    return this._lastIndexedMatch;
  },

  // Visibility check: returns true if element is visible in the page.
  //
  // For iframe-nested elements, additionally requires that the element's
  // bounding rect intersects every ancestor iframe's content viewport. An
  // element scrolled or clipped outside its iframe is reported as not
  // visible — matching what a user would see, and unblocking
  // scrollUntilVisible's iframe-internal scrollIntoView path (which relies
  // on this predicate to know when to stop scrolling).
  //
  // Top-frame "below-the-fold" elements are NOT rejected here. That matches
  // pre-existing behaviour (and Maestro CLI semantics): visibility is about
  // whether an element is rendered, not whether it currently sits inside
  // the visible portion of the top viewport.
  _isElementVisible: function(el) {
    if (!el || !el.isConnected) return false;
    // Use element's own window — getComputedStyle from a different window
    // may return null for cross-document elements.
    var ownerWin = (el.ownerDocument && el.ownerDocument.defaultView) || window;
    // Check offsetParent (null means display:none, except for body/html/fixed)
    if (el.offsetParent === null) {
      var style = ownerWin.getComputedStyle(el);
      if (!style) return false;
      if (style.display === 'none') return false;
      if (style.visibility === 'hidden') return false;
      // Fixed/sticky elements have null offsetParent but can be visible
      if (style.position !== 'fixed' && style.position !== 'sticky') {
        // Check if it's body/html
        var tag = el.tagName.toLowerCase();
        if (tag !== 'body' && tag !== 'html') return false;
      }
    }
    var rect = el.getBoundingClientRect();
    if (rect.width === 0 && rect.height === 0) return false;
    var style2 = ownerWin.getComputedStyle(el);
    if (style2 && (style2.visibility === 'hidden' || style2.opacity === '0')) return false;
    if (this._isInIframe(el)) {
      return this._intersectsIframeChain(el, rect);
    }
    return true;
  },

  // _intersectsIframeChain: walk the iframe ancestor chain and verify the
  // element's rect intersects each iframe's content viewport (innerWidth ×
  // innerHeight of the iframe's contentWindow). Returns false on the first
  // empty intersection, true once the top frame is reached.
  //
  // After each successful intersection, the surviving rect is translated to
  // the parent document's viewport coordinates using the iframe element's
  // box plus its content-area inset (mirroring topFrameClickPoint).
  // Transformed iframe ancestors short-circuit to true — same bail
  // Playwright takes (we follow that decision rather than computing through
  // DOMMatrix), and consistent with the tap path.
  _intersectsIframeChain: function(el, rect) {
    var left = rect.left, top = rect.top, right = rect.right, bottom = rect.bottom;
    var doc = el.ownerDocument;
    while (doc && doc.defaultView) {
      var w = doc.defaultView;
      var iframe;
      try { iframe = w.frameElement; } catch (e) { return true; }
      if (!iframe) return true; // reached top frame
      var vw, vh;
      try {
        vw = w.innerWidth || (doc.documentElement && doc.documentElement.clientWidth) || 0;
        vh = w.innerHeight || (doc.documentElement && doc.documentElement.clientHeight) || 0;
      } catch (e) { return true; }
      var ix1 = Math.max(0, left), iy1 = Math.max(0, top);
      var ix2 = Math.min(vw, right), iy2 = Math.min(vh, bottom);
      if (ix2 <= ix1 || iy2 <= iy1) return false;
      var style = this._describeIFrameStyle(iframe);
      if (typeof style === 'string') return true; // transformed/notconnected — bail
      var box = iframe.getBoundingClientRect();
      left   = box.left + style.left + ix1;
      top    = box.top  + style.top  + iy1;
      right  = box.left + style.left + ix2;
      bottom = box.top  + style.top  + iy2;
      doc = iframe.ownerDocument;
    }
    return true;
  },

  // Find elements matching selector config and return true if any are visible.
  // selectorType: "text", "id", "css", "textContains", "textRegex", or attribute types
  _isAnyVisible: function(selectorType, selectorValue) {
    var self = this;
    var elements = self._findMatchingElements(selectorType, selectorValue);
    for (var i = 0; i < elements.length; i++) {
      if (self._isElementVisible(elements[i])) return true;
    }
    return false;
  },

  // Filter to deepest elements: remove any element that is an ancestor of another match.
  // This ensures text-based visibility checks use the actual text-bearing element,
  // not a parent whose textContent includes hidden children's text.
  _filterToDeepest: function(elements) {
    if (elements.length <= 1) return elements;
    return elements.filter(function(el) {
      for (var i = 0; i < elements.length; i++) {
        if (elements[i] !== el && el.contains(elements[i])) return false;
      }
      return true;
    });
  },

  // Find all elements matching a selector across same-origin docs.
  _findMatchingElements: function(selectorType, selectorValue) {
    var docs = this._collectRoots();
    var results = [];
    function pushAll(nodes) {
      for (var i = 0; i < nodes.length; i++) results.push(nodes[i]);
    }
    var escaped = (selectorValue || '').replace(/"/g, '\\"');

    switch (selectorType) {
      case 'css':
        for (var d = 0; d < docs.length; d++) {
          try { pushAll(docs[d].querySelectorAll(selectorValue)); } catch (e) {}
        }
        break;
      case 'id': {
        // Mirror the Go-side findByID cascade: #id → [data-testid] → [id*=]
        // → [name=] → [aria-label=]. First strategy that hits in any frame wins.
        var idCascade = [
          function(doc) {
            try { return doc.getElementById(selectorValue); } catch (e) { return null; }
          },
          function(doc) {
            try { return doc.querySelector('[data-testid="' + escaped + '"]'); } catch (e) { return null; }
          },
          function(doc) {
            try { return doc.querySelector('[id*="' + escaped + '"]'); } catch (e) { return null; }
          },
          function(doc) {
            try { return doc.querySelector('[name="' + escaped + '"]'); } catch (e) { return null; }
          },
          function(doc) {
            try { return doc.querySelector('[aria-label="' + escaped + '"]'); } catch (e) { return null; }
          },
        ];
        for (var s = 0; s < idCascade.length && results.length === 0; s++) {
          for (var d = 0; d < docs.length; d++) {
            var el = idCascade[s](docs[d]);
            if (el) { results.push(el); break; }
          }
        }
        break;
      }
      case 'testId':
        for (var d = 0; d < docs.length; d++) {
          try { pushAll(docs[d].querySelectorAll('[data-testid="' + escaped + '"]')); } catch (e) {}
        }
        break;
      case 'placeholder':
        for (var d = 0; d < docs.length; d++) {
          try { pushAll(docs[d].querySelectorAll('[placeholder="' + escaped + '"]')); } catch (e) {}
        }
        break;
      case 'name':
        for (var d = 0; d < docs.length; d++) {
          try { pushAll(docs[d].querySelectorAll('[name="' + escaped + '"]')); } catch (e) {}
        }
        break;
      case 'href':
        for (var d = 0; d < docs.length; d++) {
          try { pushAll(docs[d].querySelectorAll('a[href*="' + escaped + '"]')); } catch (e) {}
        }
        break;
      case 'alt':
        for (var d = 0; d < docs.length; d++) {
          try { pushAll(docs[d].querySelectorAll('[alt="' + escaped + '"]')); } catch (e) {}
        }
        break;
      case 'title':
        for (var d = 0; d < docs.length; d++) {
          try { pushAll(docs[d].querySelectorAll('[title="' + escaped + '"]')); } catch (e) {}
        }
        break;
      case 'text': {
        var lower = selectorValue.toLowerCase();
        for (var d = 0; d < docs.length; d++) {
          var all;
          try { all = docs[d].querySelectorAll('*'); } catch (e) { continue; }
          for (var i = 0; i < all.length; i++) {
            var el = all[i];
            var t = (el.textContent || '').trim().toLowerCase();
            var label = (el.getAttribute('aria-label') || '').toLowerCase();
            var ph = (el.getAttribute('placeholder') || '').toLowerCase();
            if (t === lower || label === lower || ph === lower ||
                t.indexOf(lower) !== -1 || label.indexOf(lower) !== -1 || ph.indexOf(lower) !== -1) {
              results.push(el);
            }
          }
        }
        results = this._filterToDeepest(results);
        break;
      }
      case 'textContains': {
        var lower = selectorValue.toLowerCase();
        for (var d = 0; d < docs.length; d++) {
          var all;
          try { all = docs[d].querySelectorAll('*'); } catch (e) { continue; }
          for (var i = 0; i < all.length; i++) {
            var t = (all[i].textContent || '').trim().toLowerCase();
            if (t.indexOf(lower) !== -1) results.push(all[i]);
          }
        }
        results = this._filterToDeepest(results);
        break;
      }
      case 'textRegex': {
        try {
          var re = new RegExp(selectorValue, 'i');
          for (var d = 0; d < docs.length; d++) {
            var all;
            try { all = docs[d].querySelectorAll('*'); } catch (e) { continue; }
            for (var i = 0; i < all.length; i++) {
              var t = (all[i].textContent || '').trim();
              var label = all[i].getAttribute('aria-label') || '';
              if (re.test(t) || re.test(label)) results.push(all[i]);
            }
          }
        } catch (e) {}
        results = this._filterToDeepest(results);
        break;
      }
      case 'role': {
        var roleSelector = '[role="' + escaped + '"]';
        for (var d = 0; d < docs.length; d++) {
          try { pushAll(docs[d].querySelectorAll(roleSelector)); } catch (e) {}
        }
        break;
      }
    }
    return results;
  },

  // RAF-based polling: waits until no matching element is visible or timeout.
  // Returns a promise that resolves to true (element gone) or false (still visible at timeout).
  waitForNotVisible: function(selectorType, selectorValue, timeoutMs) {
    var self = this;
    return new Promise(function(resolve) {
      var deadline = Date.now() + timeoutMs;

      // Quick check: already not visible?
      if (!self._isAnyVisible(selectorType, selectorValue)) {
        resolve(true);
        return;
      }

      // RAF polling loop
      function check() {
        if (!self._isAnyVisible(selectorType, selectorValue)) {
          resolve(true);
          return;
        }
        if (Date.now() >= deadline) {
          resolve(false);
          return;
        }
        requestAnimationFrame(check);
      }
      requestAnimationFrame(check);
    });
  },

  // RAF-based polling: waits until a matching element is visible or timeout.
  // Returns a promise that resolves to true (element visible) or false (not found at timeout).
  waitForVisible: function(selectorType, selectorValue, timeoutMs) {
    var self = this;
    return new Promise(function(resolve) {
      var deadline = Date.now() + timeoutMs;

      // Quick check: already visible?
      if (self._isAnyVisible(selectorType, selectorValue)) {
        resolve(true);
        return;
      }

      // RAF polling loop
      function check() {
        if (self._isAnyVisible(selectorType, selectorValue)) {
          resolve(true);
          return;
        }
        if (Date.now() >= deadline) {
          resolve(false);
          return;
        }
        requestAnimationFrame(check);
      }
      requestAnimationFrame(check);
    });
  },

  // ─── Iframe / shadow-root click coordinate translation + hit-target verify ───
  //
  // Ports Playwright's `_checkFrameIsHitTarget` walk and `setupHitTargetInterceptor`
  // (microsoft/playwright `packages/playwright-core/src/server/dom.ts` and
  // `packages/injected/src/injectedScript.ts`) so that taps on elements nested
  // inside iframes (or iframe + open shadow root) dispatch at the correct
  // top-frame viewport coordinates and verify post-dispatch that the click
  // actually reached the target. (Issues #71/#72 acting layer.)
  //
  // Why a new path: Rod's Element.Click() uses getBoundingClientRect() which
  // returns iframe-LOCAL coordinates for elements inside an iframe. CDP
  // Input.dispatchMouseEvent operates in TOP-FRAME viewport coordinates.
  // Mismatch → click lands at the wrong place, reports success silently.

  // _parentElementOrShadowHost: walk one step up — through parent element or
  // through shadow-root host boundary. Mirrors Playwright domUtils helper of
  // the same name.
  _parentElementOrShadowHost: function(el) {
    if (!el) return null;
    if (el.parentElement) return el.parentElement;
    var root = el.getRootNode && el.getRootNode();
    // 11 = DOCUMENT_FRAGMENT_NODE; ShadowRoot is the only fragment with .host
    if (root && root.nodeType === 11 && root.host) return root.host;
    return null;
  },

  // _isInFlutterContext: walks up (piercing shadow boundaries) and returns
  // true if the node lives inside a Flutter Web app — i.e. has any ancestor
  // with a Flutter-namespaced tag (`flutter-view` or `flt-*`). Real Flutter
  // Web DOMs vary by embedder/version: in some <flutter-view> wraps both the
  // canvas and <flt-semantics-host>; in others <flt-semantics-host> is a
  // SIBLING of <flutter-view> (semantics tree rendered separately for screen
  // readers). Either layout is "inside Flutter" for hit-target purposes —
  // Flutter's pointer router intercepts events through its glass pane and
  // dispatches to semantics via its own internal hit testing.
  _isInFlutterContext: function(node) {
    var n = node;
    while (n) {
      var tag = n.tagName ? n.tagName.toLowerCase() : '';
      if (tag === 'flutter-view' || tag.indexOf('flt-') === 0) return true;
      n = this._parentElementOrShadowHost(n);
    }
    return false;
  },

  // _isInIframe: cheap check used by Go to decide whether to use the new
  // coord-translated dispatch path. True iff the element's owner document has
  // a non-null frameElement (i.e. it lives inside an iframe at any depth).
  // Top-frame elements (including those inside top-frame shadow roots) keep
  // the existing Rod Click() path — getBoundingClientRect() is already in the
  // top-frame viewport for them.
  _isInIframe: function(el) {
    if (!el) return false;
    var doc = el.ownerDocument;
    if (!doc || !doc.defaultView) return false;
    try { return !!doc.defaultView.frameElement; } catch (e) { return false; }
  },

  // _describeIFrameStyle: verbatim port of Playwright's `describeIFrameStyle`.
  // Returns the iframe's content-area inset (border + padding) within the
  // iframe element box, or 'transformed' if the iframe (or any ancestor) has
  // a CSS transform — Playwright bails on transforms rather than computing
  // through DOMMatrix; we follow that decision so a transformed-iframe page
  // surfaces a clear error instead of a subtly-wrong click.
  _describeIFrameStyle: function(iframe) {
    if (!iframe.ownerDocument || !iframe.ownerDocument.defaultView)
      return 'error:notconnected';
    var defaultView = iframe.ownerDocument.defaultView;
    for (var e = iframe; e; e = this._parentElementOrShadowHost(e)) {
      try {
        if (defaultView.getComputedStyle(e).transform !== 'none') return 'transformed';
      } catch (err) { /* getComputedStyle on detached element throws */ }
    }
    var s = defaultView.getComputedStyle(iframe);
    return {
      left: parseInt(s.borderLeftWidth || '', 10) + parseInt(s.paddingLeft || '', 10),
      top:  parseInt(s.borderTopWidth  || '', 10) + parseInt(s.paddingTop  || '', 10)
    };
  },

  // topFrameClickPoint: translate an element's center point from iframe-local
  // viewport coordinates into top-frame viewport coordinates. Walks up the
  // frame chain; at each level adds the iframe element's box (its position in
  // the parent document) plus the iframe's content-area inset.
  //
  // Inverse direction of Playwright's `_checkFrameIsHitTarget` (which starts
  // with top-frame coords and SUBTRACTS to find frame-local coords for
  // occlusion checks at each level). We start frame-local and ADD.
  //
  // Throws on transformed-iframe ancestors (see _describeIFrameStyle).
  topFrameClickPoint: function(el) {
    var r = el.getBoundingClientRect();
    var pt = { x: r.left + r.width / 2, y: r.top + r.height / 2 };
    var doc = el.ownerDocument;
    while (doc && doc.defaultView) {
      var iframe;
      try { iframe = doc.defaultView.frameElement; } catch (e) { break; }
      if (!iframe) break;
      var style = this._describeIFrameStyle(iframe);
      if (typeof style === 'string') {
        throw new Error('iframe coord translation: ' + style + ' iframe ancestor');
      }
      var box = iframe.getBoundingClientRect();
      pt.x += box.left + style.left;
      pt.y += box.top + style.top;
      doc = iframe.ownerDocument;
    }
    return pt;
  },

  // _previewNode: short string description of a node, used inside
  // hitTargetDescription for actionable error messages. Best-effort, never
  // throws.
  _previewNode: function(node) {
    if (!node) return '<none>';
    try {
      var tag = (node.tagName || node.nodeName || '').toLowerCase();
      var id = node.id ? '#' + node.id : '';
      var cls = '';
      if (node.classList && node.classList.length) {
        cls = '.' + Array.prototype.slice.call(node.classList).slice(0, 2).join('.');
      }
      var text = '';
      if (node.textContent) {
        var t = node.textContent.trim().replace(/\s+/g, ' ');
        if (t) text = ' "' + (t.length > 30 ? t.slice(0, 30) + '…' : t) + '"';
      }
      return '<' + tag + id + cls + '>' + text;
    } catch (e) { return '<node>'; }
  },

  // expectHitTarget: verbatim port of Playwright's `expectHitTarget` from
  // injectedScript.ts. Walks shadow-root boundaries via elementsFromPoint per
  // root (document.elementFromPoint does NOT pierce shadow boundaries — only
  // sees the host). Returns 'done' if the click point would land on the
  // target (or one of its descendants reachable through slot/host chain), or
  // { hitTargetDescription } if something is in the way.
  expectHitTarget: function(hitPoint, targetElement) {
    var roots = [];
    var parentElement = targetElement;
    while (parentElement) {
      var root = parentElement.getRootNode && parentElement.getRootNode();
      if (!root) break;
      roots.push(root);
      if (root.nodeType === 9) break; // 9 = DOCUMENT_NODE; reached top
      parentElement = root.host; // ShadowRoot.host
    }
    var hitElement;
    for (var index = roots.length - 1; index >= 0; index--) {
      var r = roots[index];
      var elements;
      var single;
      try {
        elements = r.elementsFromPoint(hitPoint.x, hitPoint.y);
        single = r.elementFromPoint(hitPoint.x, hitPoint.y);
      } catch (e) { break; }
      if (single && elements && elements[0] &&
          this._parentElementOrShadowHost(single) === elements[0]) {
        try {
          var st = window.getComputedStyle(single);
          if (st && st.display === 'contents') elements.unshift(single);
        } catch (e) {}
      }
      if (elements && elements[0] && elements[0].shadowRoot === r &&
          elements[1] === single) {
        elements.shift();
      }
      var innerElement = elements && elements[0];
      if (!innerElement) break;
      hitElement = innerElement;
      // If we're not at the deepest root yet, the hit element should be the
      // shadow host that opens the next root down. If it isn't, occlusion at
      // an outer level — stop walking deeper.
      if (index && innerElement !== (roots[index - 1] && roots[index - 1].host)) break;
    }
    var hitParents = [];
    while (hitElement && hitElement !== targetElement) {
      hitParents.push(hitElement);
      hitElement = hitElement.assignedSlot || this._parentElementOrShadowHost(hitElement);
    }
    if (hitElement === targetElement) return 'done';
    // Flutter Web concession: <flutter-view> (and the legacy <flt-glass-pane>)
    // acts as a glass pane that intercepts every pointer event and routes it
    // to the appropriate <flt-semantics> action via Flutter's internal hit
    // testing. The accessibility tree may live in light DOM under flutter-
    // view, in its shadow root, or as a sibling <flt-semantics-host> with the
    // rendering canvas stacked above — either way DOM elementsFromPoint at a
    // semantics target returns the canvas / glass pane, never the semantics
    // node. A strict same-element walk-up would therefore always report false
    // occlusion and refuse to dispatch. Accept when both target and the hit
    // element are inside the same Flutter app's iframe — Flutter will route
    // the trusted click to the right semantics action.
    var topHit = hitParents[0];
    if (topHit && this._isInFlutterContext(targetElement) &&
        this._isInFlutterContext(topHit) &&
        targetElement.ownerDocument === topHit.ownerDocument) {
      return 'done';
    }
    var description = this._previewNode(topHit || document.documentElement);
    return { hitTargetDescription: description };
  },

  // Hit-target interceptor state. _hitTargetState[token] receives the verify
  // result captured from the trusted event listener. One entry per in-flight
  // tapOn (we only ever have one at a time in maestro flows, but keying by
  // token keeps the state hygienic).
  _hitTargetState: {},
  _hitTargetNextToken: 1,

  // _hitPointInTargetFrame: convert a top-frame viewport point to the
  // target's owner-document viewport (the inverse of topFrameClickPoint).
  // expectHitTarget always runs in the target's frame and expects coords in
  // that frame's viewport — for iframe targets this means subtracting each
  // ancestor iframe's box + content-area inset.
  _hitPointInTargetFrame: function(topPoint, targetElement) {
    var pt = { x: topPoint.x, y: topPoint.y };
    var doc = targetElement.ownerDocument;
    var frames = [];
    while (doc && doc.defaultView) {
      var iframe;
      try { iframe = doc.defaultView.frameElement; } catch (e) { break; }
      if (!iframe) break;
      frames.push(iframe);
      doc = iframe.ownerDocument;
    }
    // frames[0] = innermost, frames[last] = outermost. Walk outermost inward,
    // subtracting each iframe's box.left/top + content-area inset.
    for (var i = frames.length - 1; i >= 0; i--) {
      var iframe = frames[i];
      var style = this._describeIFrameStyle(iframe);
      if (typeof style === 'string') break; // transformed/notconnected — bail
      var box = iframe.getBoundingClientRect();
      pt.x -= box.left + style.left;
      pt.y -= box.top + style.top;
    }
    return pt;
  },

  // setupHitTargetInterceptor: install a one-shot listener that captures the
  // ACTUAL clientX/Y of the first trusted pointerdown/mousedown event after
  // setup, runs expectHitTarget against that point, and stashes the result
  // for later polling by Go.
  //
  // Pre-flight: expectHitTarget is also run synchronously here against the
  // computed hitPoint; if pre-flight fails (occluded, wrong element on top),
  // we return { error } immediately so the caller skips dispatch.
  //
  // Why listen to the trusted event instead of just verifying after Click():
  // by the time Click() returns, the DOM may have already mutated (e.g. the
  // dialog handler removes the dialog). Capturing at event-fire time gives
  // a snapshot of what was actually under the pointer when the click landed.
  // Same pattern as Playwright's `setupHitTargetInterceptor`.
  //
  // Frame scope: the listener is installed on the target's owner-document
  // window. Pointer events don't cross frame boundaries, so a top-window
  // listener wouldn't see clicks landing inside an iframe.
  setupHitTargetInterceptor: function(targetElement, hitPoint) {
    // hitPoint comes in TOP-FRAME viewport coords (as computed by
    // topFrameClickPoint). expectHitTarget runs in the target's frame, so
    // convert before passing.
    var localPoint = this._hitPointInTargetFrame(hitPoint, targetElement);
    var pre = this.expectHitTarget(localPoint, targetElement);
    if (pre !== 'done') return { error: pre.hitTargetDescription };

    var self = this;
    var token = this._hitTargetNextToken++;
    var win = targetElement.ownerDocument && targetElement.ownerDocument.defaultView;
    if (!win) return { error: '<target window unavailable>' };
    var inFlutter = this._isInFlutterContext(targetElement);
    var state = {
      captured: false, result: undefined, target: targetElement, win: win,
      inFlutter: inFlutter
    };
    this._hitTargetState[token] = state;

    var listener = function(ev) {
      if (state.captured) return;
      if (!ev.isTrusted) return;
      if (ev.type !== 'pointerdown' && ev.type !== 'mousedown') return;
      // Only interested in primary (left) button.
      if (typeof ev.button === 'number' && ev.button !== 0) return;
      state.captured = true;
      try {
        // ev.clientX/Y are in the firing window's viewport == target's frame.
        state.result = self.expectHitTarget({ x: ev.clientX, y: ev.clientY }, state.target);
      } catch (e) {
        state.result = { hitTargetDescription: '<error: ' + (e && e.message) + '>' };
      }
      // Listener is one-shot — remove ourselves to avoid leaking handlers.
      try {
        state.win.removeEventListener('pointerdown', listener, true);
        state.win.removeEventListener('mousedown', listener, true);
      } catch (e) {}
    };
    state.listener = listener;
    win.addEventListener('pointerdown', listener, true);
    win.addEventListener('mousedown', listener, true);
    return { token: token };
  },

  // pollHitTargetResult: returns the captured verify outcome for a token.
  // Caller should poll a few times after dispatch (a real trusted pointerdown
  // is delivered synchronously to JS during Mouse.Click, so 'pending' should
  // be rare — but a brief retry window absorbs scheduler jitter).
  //
  // Always returns an object with a `status` field:
  //   { status: 'done' }                                 → success
  //   { status: 'pending', inFlutter: bool }             → trusted event not seen yet
  //   { status: 'failed', hitTargetDescription: string } → landed on the wrong element
  //
  // After returning a non-'pending' result, the token is cleaned up.
  pollHitTargetResult: function(token) {
    var state = this._hitTargetState[token];
    if (!state) return { status: 'done' }; // unknown token → don't block caller
    if (!state.captured) return { status: 'pending', inFlutter: !!state.inFlutter };
    var r = state.result;
    // Cleanup listener if still attached (defensive — listener removes itself
    // on capture, but if the click never fired we want to stop leaking).
    try {
      state.win.removeEventListener('pointerdown', state.listener, true);
      state.win.removeEventListener('mousedown', state.listener, true);
    } catch (e) {}
    delete this._hitTargetState[token];
    // Normalize the listener-stashed expectHitTarget result into the unified
    // shape. expectHitTarget returns 'done' or { hitTargetDescription }.
    if (r === 'done') return { status: 'done' };
    if (r && typeof r === 'object' && 'hitTargetDescription' in r) {
      return { status: 'failed', hitTargetDescription: r.hitTargetDescription };
    }
    return { status: 'done' }; // defensive fallback
  },

  // disposeHitTargetInterceptor: called by Go to abandon a token without
  // waiting for a result (e.g. Mouse.Click failed before any event fired).
  // Removes the listener and frees the slot.
  disposeHitTargetInterceptor: function(token) {
    var state = this._hitTargetState[token];
    if (!state) return;
    try {
      state.win.removeEventListener('pointerdown', state.listener, true);
      state.win.removeEventListener('mousedown', state.listener, true);
    } catch (e) {}
    delete this._hitTargetState[token];
  }
};

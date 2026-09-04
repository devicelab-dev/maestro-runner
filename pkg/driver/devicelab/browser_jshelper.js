// SPDX-FileCopyrightText: 2026 DeviceLab and the Project Contributors
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

(function() {
  // Override alert/confirm/prompt — on page-level Chrome Android CDP connections,
  // a native dialog freezes ALL CDP commands including Page.handleJavaScriptDialog.
  window.alert = function(msg) { console.log('[maestro] alert suppressed: ' + msg); };
  window.confirm = function(msg) { console.log('[maestro] confirm suppressed: ' + msg); return true; };
  window.prompt = function(msg, def) { console.log('[maestro] prompt suppressed: ' + msg); return def || ''; };

window.__maestro = {
  findByText: function(text) {
    var lower = text.toLowerCase();
    var all = document.querySelectorAll('*');
    var best = null, bestDepth = -1;
    for (var i = 0; i < all.length; i++) {
      var el = all[i];
      var t = (el.textContent || '').trim().toLowerCase();
      var label = (el.getAttribute('aria-label') || '').toLowerCase();
      var ph = (el.getAttribute('placeholder') || '').toLowerCase();
      if (t.indexOf(lower) !== -1 || label.indexOf(lower) !== -1 || ph.indexOf(lower) !== -1) {
        var d = 0, n = el;
        while (n.parentElement) { d++; n = n.parentElement; }
        if (d > bestDepth) { best = el; bestDepth = d; }
      }
    }
    if (!best) throw new Error('not found: ' + text);
    var p = best;
    while (p && p !== document.body) {
      var tag = p.tagName.toLowerCase();
      if (['a','button','input','select','textarea'].indexOf(tag) !== -1 ||
          p.getAttribute('role') === 'button' || p.getAttribute('tabindex') !== null) return p;
      p = p.parentElement;
    }
    return best;
  },

  _isElementVisible: function(el) {
    if (!el || !el.isConnected) return false;
    if (el.offsetParent === null) {
      var style = window.getComputedStyle(el);
      if (style.display === 'none') return false;
      if (style.visibility === 'hidden') return false;
      if (style.position !== 'fixed' && style.position !== 'sticky') {
        var tag = el.tagName.toLowerCase();
        if (tag !== 'body' && tag !== 'html') return false;
      }
    }
    var rect = el.getBoundingClientRect();
    if (rect.width === 0 && rect.height === 0) return false;
    var style = window.getComputedStyle(el);
    if (style.visibility === 'hidden' || style.opacity === '0') return false;
    return true;
  },

  _findMatchingElements: function(selectorType, selectorValue) {
    var results = [];
    switch (selectorType) {
      case 'css':
        try { results = Array.from(document.querySelectorAll(selectorValue)); } catch(e) {}
        break;
      case 'id':
        var el = document.getElementById(selectorValue);
        if (el) results = [el];
        break;
      case 'testId':
        results = Array.from(document.querySelectorAll('[data-testid="' + selectorValue.replace(/"/g, '\\"') + '"]'));
        break;
      case 'placeholder':
        results = Array.from(document.querySelectorAll('[placeholder="' + selectorValue.replace(/"/g, '\\"') + '"]'));
        break;
      case 'name':
        results = Array.from(document.querySelectorAll('[name="' + selectorValue.replace(/"/g, '\\"') + '"]'));
        break;
      case 'href':
        results = Array.from(document.querySelectorAll('a[href*="' + selectorValue.replace(/"/g, '\\"') + '"]'));
        break;
      case 'alt':
        results = Array.from(document.querySelectorAll('[alt="' + selectorValue.replace(/"/g, '\\"') + '"]'));
        break;
      case 'title':
        results = Array.from(document.querySelectorAll('[title="' + selectorValue.replace(/"/g, '\\"') + '"]'));
        break;
      case 'text': {
        var lower = selectorValue.toLowerCase();
        var all = document.querySelectorAll('*');
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
        break;
      }
      case 'textContains': {
        var lower = selectorValue.toLowerCase();
        var all = document.querySelectorAll('*');
        for (var i = 0; i < all.length; i++) {
          var t = (all[i].textContent || '').trim().toLowerCase();
          if (t.indexOf(lower) !== -1) results.push(all[i]);
        }
        break;
      }
      case 'textRegex': {
        try {
          var re = new RegExp(selectorValue, 'i');
          var all = document.querySelectorAll('*');
          for (var i = 0; i < all.length; i++) {
            var t = (all[i].textContent || '').trim();
            var label = all[i].getAttribute('aria-label') || '';
            if (re.test(t) || re.test(label)) results.push(all[i]);
          }
        } catch(e) {}
        break;
      }
      case 'role': {
        var roleSelector = '[role="' + selectorValue.replace(/"/g, '\\"') + '"]';
        results = Array.from(document.querySelectorAll(roleSelector));
        break;
      }
    }
    return results;
  },

  _isAnyVisible: function(selectorType, selectorValue) {
    var self = this;
    var elements = self._findMatchingElements(selectorType, selectorValue);
    for (var i = 0; i < elements.length; i++) {
      if (self._isElementVisible(elements[i])) return true;
    }
    return false;
  },

  // Find first visible element matching selector. Returns the element or null.
  findVisible: function(selectorType, selectorValue) {
    var self = this;
    var elements = self._findMatchingElements(selectorType, selectorValue);
    // Pick deepest visible element (most specific)
    var best = null, bestDepth = -1;
    for (var i = 0; i < elements.length; i++) {
      if (!self._isElementVisible(elements[i])) continue;
      var d = 0, n = elements[i];
      while (n.parentElement) { d++; n = n.parentElement; }
      if (d > bestDepth) { best = elements[i]; bestDepth = d; }
    }
    return best;
  },

  waitForNotVisible: function(selectorType, selectorValue, timeoutMs) {
    var self = this;
    return new Promise(function(resolve) {
      var deadline = Date.now() + timeoutMs;
      if (!self._isAnyVisible(selectorType, selectorValue)) {
        resolve(true);
        return;
      }
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

  waitForVisible: function(selectorType, selectorValue, timeoutMs) {
    var self = this;
    return new Promise(function(resolve) {
      var deadline = Date.now() + timeoutMs;
      if (self._isAnyVisible(selectorType, selectorValue)) {
        resolve(true);
        return;
      }
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
  }
};
})();

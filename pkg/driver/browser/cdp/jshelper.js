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

  // Visibility check: returns true if element is visible in the page.
  _isElementVisible: function(el) {
    if (!el || !el.isConnected) return false;
    // Check offsetParent (null means display:none, except for body/html/fixed)
    if (el.offsetParent === null) {
      var style = window.getComputedStyle(el);
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
    var style = window.getComputedStyle(el);
    if (style.visibility === 'hidden' || style.opacity === '0') return false;
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

  // Find all elements matching a selector.
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
  }
};

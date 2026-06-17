// SPDX-FileCopyrightText: 2024 DeviceLab
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

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
  }
};

package cdp

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// findElement finds an element using the selector with polling until timeout.
// Uses Rod's clone-based timeout: creates a new page object with deadline.
func (d *Driver) findElement(sel flow.Selector, optional bool, stepTimeoutMs int) (*rod.Element, *core.ElementInfo, error) {
	// Warn about unsupported selector fields (once per field)
	if unsupported := flow.CheckUnsupportedFields(&sel, "web"); len(unsupported) > 0 {
		for _, field := range unsupported {
			if !d.warnedFields[field] {
				d.warnedFields[field] = true
				log.Printf("[browser] warning: %q is not supported on web — will be ignored", field)
			}
		}
	}

	timeout := d.calculateTimeout(optional, stepTimeoutMs)
	deadline := time.Now().Add(timeout)

	var lastErr error
	for time.Now().Before(deadline) {
		elem, info, err := d.findElementOnce(sel)
		if err == nil {
			return elem, info, nil
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}

	if lastErr != nil {
		return nil, nil, fmt.Errorf("element '%s' not found within %v: %w", sel.Describe(), timeout, lastErr)
	}
	return nil, nil, fmt.Errorf("element '%s' not found within %v", sel.Describe(), timeout)
}

// findElementOnce performs a single attempt to find an element (no polling).
// Priority: CSS > attribute selectors > role > ID > text matching.
func (d *Driver) findElementOnce(sel flow.Selector) (*rod.Element, *core.ElementInfo, error) {
	switch {
	// 1. CSS (most specific)
	case sel.CSS != "":
		return d.findByCSS(sel)

	// 2. Attribute selectors
	case sel.TestID != "":
		return d.findByAttribute("data-testid", sel.TestID, sel)
	case sel.Name != "":
		return d.findByAttribute("name", sel.Name, sel)
	case sel.Placeholder != "":
		return d.findByAttribute("placeholder", sel.Placeholder, sel)
	case sel.Href != "":
		return d.findByHref(sel)
	case sel.Alt != "":
		return d.findByAttribute("alt", sel.Alt, sel)
	case sel.Title != "":
		return d.findByAttribute("title", sel.Title, sel)

	// 3. Role (+ optional text combination)
	case sel.Role != "":
		return d.findByRole(sel)

	// 4. ID cascade
	case sel.ID != "":
		return d.findByID(sel)

	// 5. Text matching
	case sel.Text != "":
		return d.findByText(sel)
	case sel.TextContains != "":
		return d.findByTextContains(sel)
	case sel.TextRegex != "":
		return d.findByTextRegex(sel)

	default:
		return nil, nil, fmt.Errorf("no selector specified")
	}
}

// findByCSS finds an element by CSS selector.
func (d *Driver) findByCSS(sel flow.Selector) (*rod.Element, *core.ElementInfo, error) {
	if sel.Nth > 0 {
		p := d.page.Timeout(2 * time.Second)
		elems, err := p.Elements(sel.CSS)
		if err != nil {
			return nil, nil, fmt.Errorf("CSS selector '%s' not found: %w", sel.CSS, err)
		}
		if sel.Nth >= len(elems) {
			return nil, nil, fmt.Errorf("CSS selector '%s': nth=%d but only %d elements found", sel.CSS, sel.Nth, len(elems))
		}
		elem := elems[sel.Nth]
		if !d.matchesStateFilters(elem, sel) {
			return nil, nil, fmt.Errorf("CSS selector '%s' found (nth=%d) but state filters don't match", sel.CSS, sel.Nth)
		}
		info := d.elementInfo(elem)
		return elem, info, nil
	}

	p := d.page.Timeout(2 * time.Second)
	elem, err := p.Element(sel.CSS)
	if err != nil {
		return nil, nil, fmt.Errorf("CSS selector '%s' not found: %w", sel.CSS, err)
	}

	if !d.matchesStateFilters(elem, sel) {
		return nil, nil, fmt.Errorf("CSS selector '%s' found but state filters don't match", sel.CSS)
	}

	info := d.elementInfo(elem)
	return elem, info, nil
}

// findByID finds an element by ID using a cascade of strategies.
// Uses one-shot lookups (NotFoundSleeper) so failed strategies don't waste time retrying.
func (d *Driver) findByID(sel flow.Selector) (*rod.Element, *core.ElementInfo, error) {
	selectors := []string{
		"#" + cssEscape(sel.ID),
		fmt.Sprintf("[data-testid=%q]", sel.ID),
		fmt.Sprintf("[id*=%q]", sel.ID),
		fmt.Sprintf("[name=%q]", sel.ID),
		fmt.Sprintf("[aria-label=%q]", sel.ID),
	}

	// One-shot: return immediately if not found instead of retrying
	p := d.page.Sleeper(rod.NotFoundSleeper)
	for _, css := range selectors {
		elem, err := p.Element(css)
		if err != nil {
			continue
		}
		if !d.matchesStateFilters(elem, sel) {
			continue
		}
		info := d.elementInfo(elem)
		return elem, info, nil
	}

	return nil, nil, fmt.Errorf("element with id '%s' not found", sel.ID)
}

// findByText finds an element by text content using a multi-stage cascade:
// 1. AX tree query (clickable roles first, then input roles, then all roles)
// 2. Rod page.Search() fallback (Shadow DOM support)
// 3. JS fallback (last resort)
func (d *Driver) findByText(sel flow.Selector) (*rod.Element, *core.ElementInfo, error) {
	text := sel.Text

	// If text looks like a regex pattern, delegate to regex matching
	if looksLikeRegex(text) {
		regexSel := sel
		regexSel.TextRegex = text
		regexSel.Text = ""
		return d.findByTextRegex(regexSel)
	}

	// Stage 1a: AX tree — clickable roles
	clickableRoles := []string{"button", "link", "menuitem", "tab", "checkbox", "radio"}
	for _, role := range clickableRoles {
		elem, info, err := d.findByAXTree(text, role, sel)
		if err == nil {
			return elem, info, nil
		}
	}

	// Stage 1b: AX tree — input roles
	inputRoles := []string{"textbox", "combobox", "searchbox", "spinbutton"}
	for _, role := range inputRoles {
		elem, info, err := d.findByAXTree(text, role, sel)
		if err == nil {
			return elem, info, nil
		}
	}

	// Stage 1c: AX tree — no role filter (all roles)
	elem, info, err := d.findByAXTree(text, "", sel)
	if err == nil {
		return elem, info, nil
	}

	// Stage 2: Rod page.Search() — handles Shadow DOM
	elem, info, err = d.findBySearch(text, sel)
	if err == nil {
		return elem, info, nil
	}

	// Stage 3: JS fallback — last resort
	elem, info, err = d.findByJS(text, sel)
	if err == nil {
		return elem, info, nil
	}

	return nil, nil, fmt.Errorf("element with text '%s' not found", text)
}

// findByAttribute finds an element by a CSS attribute selector (exact match).
func (d *Driver) findByAttribute(attr, value string, sel flow.Selector) (*rod.Element, *core.ElementInfo, error) {
	css := fmt.Sprintf("[%s=%q]", attr, value)
	return d.findByCSSWithNth(css, sel, fmt.Sprintf("%s '%s'", attr, value))
}

// findByHref finds a link element by href attribute.
// Tries exact match first, then partial (contains) match.
func (d *Driver) findByHref(sel flow.Selector) (*rod.Element, *core.ElementInfo, error) {
	// Exact match
	css := fmt.Sprintf("[href=%q]", sel.Href)
	elem, info, err := d.findByCSSWithNth(css, sel, fmt.Sprintf("href '%s'", sel.Href))
	if err == nil {
		return elem, info, nil
	}

	// Partial match (contains)
	css = fmt.Sprintf("[href*=%q]", sel.Href)
	return d.findByCSSWithNth(css, sel, fmt.Sprintf("href contains '%s'", sel.Href))
}

// findByRole finds an element by ARIA role using the accessibility tree.
// If sel.Text is also set, filters by accessible name + role.
func (d *Driver) findByRole(sel flow.Selector) (*rod.Element, *core.ElementInfo, error) {
	text := sel.Text
	return d.findByAXTree(text, sel.Role, sel)
}

// findByTextContains finds elements whose accessible name contains the given substring.
func (d *Driver) findByTextContains(sel flow.Selector) (*rod.Element, *core.ElementInfo, error) {
	text := sel.TextContains

	// Query AX tree for all nodes (no name filter), then match by contains
	body, err := d.page.Timeout(2 * time.Second).Element("body")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get body: %w", err)
	}

	query := &proto.AccessibilityQueryAXTree{
		ObjectID: body.Object.ObjectID,
	}
	result, err := query.Call(d.page)
	if err != nil {
		return nil, nil, fmt.Errorf("AX tree query failed: %w", err)
	}

	var matches []*proto.AccessibilityAXNode
	for i := range result.Nodes {
		node := result.Nodes[i]
		if node.Name != nil && strings.Contains(node.Name.Value.Str(), text) {
			matches = append(matches, node)
		}
	}

	if len(matches) == 0 {
		// Fallback: JS textContent contains
		return d.findByJSTextContains(text, sel)
	}

	return d.resolveAXNodes(matches, sel, fmt.Sprintf("textContains '%s'", text))
}

// findByTextRegex finds elements whose accessible name matches a regex pattern.
func (d *Driver) findByTextRegex(sel flow.Selector) (*rod.Element, *core.ElementInfo, error) {
	re, err := regexp.Compile(sel.TextRegex)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid textRegex pattern %q: %w", sel.TextRegex, err)
	}

	// Query AX tree for all nodes, filter by regex
	body, err := d.page.Timeout(2 * time.Second).Element("body")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get body: %w", err)
	}

	query := &proto.AccessibilityQueryAXTree{
		ObjectID: body.Object.ObjectID,
	}
	result, err := query.Call(d.page)
	if err != nil {
		return nil, nil, fmt.Errorf("AX tree query failed: %w", err)
	}

	var matches []*proto.AccessibilityAXNode
	for i := range result.Nodes {
		node := result.Nodes[i]
		if node.Name != nil && re.MatchString(node.Name.Value.Str()) {
			matches = append(matches, node)
		}
	}

	if len(matches) == 0 {
		// Fallback: JS textContent regex match
		return d.findByJSTextRegex(sel.TextRegex, re, sel)
	}

	return d.resolveAXNodes(matches, sel, fmt.Sprintf("textRegex '%s'", sel.TextRegex))
}

// findByCSSWithNth finds elements by CSS selector, applying nth selection if set.
func (d *Driver) findByCSSWithNth(css string, sel flow.Selector, desc string) (*rod.Element, *core.ElementInfo, error) {
	p := d.page.Sleeper(rod.NotFoundSleeper)

	if sel.Nth > 0 {
		elems, err := p.Elements(css)
		if err != nil {
			return nil, nil, fmt.Errorf("%s not found: %w", desc, err)
		}
		if sel.Nth >= len(elems) {
			return nil, nil, fmt.Errorf("%s: nth=%d but only %d elements found", desc, sel.Nth, len(elems))
		}
		elem := elems[sel.Nth]
		if !d.matchesStateFilters(elem, sel) {
			return nil, nil, fmt.Errorf("%s found (nth=%d) but state filters don't match", desc, sel.Nth)
		}
		info := d.elementInfo(elem)
		return elem, info, nil
	}

	elem, err := p.Element(css)
	if err != nil {
		return nil, nil, fmt.Errorf("%s not found: %w", desc, err)
	}
	if !d.matchesStateFilters(elem, sel) {
		return nil, nil, fmt.Errorf("%s found but state filters don't match", desc)
	}
	info := d.elementInfo(elem)
	return elem, info, nil
}

// resolveAXNodes resolves a list of AX nodes to a visible element, applying nth and state filters.
func (d *Driver) resolveAXNodes(nodes []*proto.AccessibilityAXNode, sel flow.Selector, desc string) (*rod.Element, *core.ElementInfo, error) {
	var visibleIdx int
	for _, node := range nodes {
		if node.BackendDOMNodeID == 0 {
			continue
		}
		elem, err := d.axNodeToElement(node)
		if err != nil {
			continue
		}
		visible, err := elem.Visible()
		if err != nil || !visible {
			continue
		}
		if !d.matchesStateFiltersFromAXNode(node, sel) {
			continue
		}

		// Apply nth filter
		if sel.Nth > 0 && visibleIdx < sel.Nth {
			visibleIdx++
			continue
		}

		info := d.elementInfo(elem)
		return elem, info, nil
	}
	return nil, nil, fmt.Errorf("no visible element found for %s", desc)
}

// findByJSTextContains finds elements using JS textContent contains as fallback.
func (d *Driver) findByJSTextContains(text string, sel flow.Selector) (*rod.Element, *core.ElementInfo, error) {
	jsCode := `(text) => {
		const all = document.querySelectorAll('*');
		for (const el of all) {
			if (el.children.length === 0 && el.textContent && el.textContent.includes(text)) {
				return el;
			}
		}
		return null;
	}`
	obj, err := d.page.Evaluate(rod.Eval(jsCode, text).ByObject())
	if err != nil {
		return nil, nil, fmt.Errorf("JS textContains failed: %w", err)
	}
	elem, err := d.page.ElementFromObject(obj)
	if err != nil {
		return nil, nil, fmt.Errorf("JS textContains: element from object failed: %w", err)
	}
	if !d.matchesStateFilters(elem, sel) {
		return nil, nil, fmt.Errorf("JS textContains found element but state filters don't match")
	}
	info := d.elementInfo(elem)
	return elem, info, nil
}

// findByJSTextRegex finds elements using JS textContent regex as fallback.
func (d *Driver) findByJSTextRegex(pattern string, re *regexp.Regexp, sel flow.Selector) (*rod.Element, *core.ElementInfo, error) {
	_ = re // regex already validated by caller
	jsCode := `(pattern) => {
		const re = new RegExp(pattern);
		const all = document.querySelectorAll('*');
		for (const el of all) {
			if (el.children.length === 0 && el.textContent && re.test(el.textContent)) {
				return el;
			}
		}
		return null;
	}`
	obj, err := d.page.Evaluate(rod.Eval(jsCode, pattern).ByObject())
	if err != nil {
		return nil, nil, fmt.Errorf("JS textRegex failed: %w", err)
	}
	elem, err := d.page.ElementFromObject(obj)
	if err != nil {
		return nil, nil, fmt.Errorf("JS textRegex: element from object failed: %w", err)
	}
	if !d.matchesStateFilters(elem, sel) {
		return nil, nil, fmt.Errorf("JS textRegex found element but state filters don't match")
	}
	info := d.elementInfo(elem)
	return elem, info, nil
}

// findByAXTree queries the accessibility tree via CDP for elements matching text and role.
func (d *Driver) findByAXTree(text, role string, sel flow.Selector) (*rod.Element, *core.ElementInfo, error) {
	// Get the document body's remote object
	body, err := d.page.Timeout(2 * time.Second).Element("body")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get body: %w", err)
	}
	query := &proto.AccessibilityQueryAXTree{
		ObjectID:       body.Object.ObjectID,
		AccessibleName: text,
	}
	if role != "" {
		query.Role = role
	}

	result, err := query.Call(d.page)
	if err != nil {
		return nil, nil, fmt.Errorf("AX tree query failed: %w", err)
	}

	if len(result.Nodes) == 0 {
		return nil, nil, fmt.Errorf("no AX nodes found for text '%s' role '%s'", text, role)
	}

	// Try each AX node until we find a visible one that matches state filters
	for _, node := range result.Nodes {
		if node.BackendDOMNodeID == 0 {
			continue
		}

		elem, err := d.axNodeToElement(node)
		if err != nil {
			continue
		}

		visible, err := elem.Visible()
		if err != nil {
			log.Printf("[browser] findByAXTree: Visible() check failed: %v", err)
			continue
		}
		if !visible {
			continue
		}

		if !d.matchesStateFiltersFromAXNode(node, sel) {
			continue
		}

		info := d.elementInfo(elem)
		return elem, info, nil
	}

	return nil, nil, fmt.Errorf("no visible AX node found for text '%s' role '%s'", text, role)
}

// findBySearch uses Rod's page.Search() which handles Shadow DOM via DOMPerformSearch.
func (d *Driver) findBySearch(text string, sel flow.Selector) (*rod.Element, *core.ElementInfo, error) {
	p := d.page.Timeout(2 * time.Second)
	res, err := p.Search(text)
	if err != nil {
		return nil, nil, fmt.Errorf("search failed: %w", err)
	}
	defer res.Release()

	if res.First == nil {
		return nil, nil, fmt.Errorf("search result empty for text '%s'", text)
	}

	elem := res.First
	if !d.matchesStateFilters(elem, sel) {
		return nil, nil, fmt.Errorf("search found element but state filters don't match")
	}

	info := d.elementInfo(elem)
	return elem, info, nil
}

// findByJS uses the injected JS helper as a last resort.
func (d *Driver) findByJS(text string, sel flow.Selector) (*rod.Element, *core.ElementInfo, error) {
	obj, err := d.page.Evaluate(rod.Eval(`(text) => window.__maestro.findByText(text)`, text).ByObject())
	if err != nil {
		return nil, nil, fmt.Errorf("JS findByText failed: %w", err)
	}

	elem, err := d.page.ElementFromObject(obj)
	if err != nil {
		return nil, nil, fmt.Errorf("JS findByText: failed to get element from object: %w", err)
	}

	if !d.matchesStateFilters(elem, sel) {
		return nil, nil, fmt.Errorf("JS found element but state filters don't match")
	}

	info := d.elementInfo(elem)
	return elem, info, nil
}

// axNodeToElement converts an AX tree node to a Rod Element.
// Handles #text nodes by auto-walking up to parent element.
func (d *Driver) axNodeToElement(node *proto.AccessibilityAXNode) (*rod.Element, error) {
	resolve := &proto.DOMResolveNode{
		BackendNodeID: node.BackendDOMNodeID,
	}
	remote, err := resolve.Call(d.page)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve node: %w", err)
	}

	elem, err := d.page.ElementFromObject(remote.Object)
	if err != nil {
		return nil, fmt.Errorf("failed to get element from object: %w", err)
	}

	return elem, nil
}

// matchesStateFilters checks if a Rod element matches the selector's state filters.
func (d *Driver) matchesStateFilters(elem *rod.Element, sel flow.Selector) bool {
	if sel.Enabled != nil {
		disabled, err := elem.Attribute("disabled")
		if err != nil {
			log.Printf("[browser] matchesStateFilters: Attribute(disabled) failed: %v", err)
		}
		isEnabled := disabled == nil
		if isEnabled != *sel.Enabled {
			return false
		}
	}
	if sel.Checked != nil {
		checked, err := elem.Property("checked")
		if err == nil {
			isChecked := checked.Bool()
			if isChecked != *sel.Checked {
				return false
			}
		}
	}
	if sel.Focused != nil {
		// Check if element is focused using Evaluate
		isFocused, err := elem.Eval(`() => document.activeElement === this`)
		if err == nil {
			if isFocused.Value.Bool() != *sel.Focused {
				return false
			}
		}
	}
	if sel.Selected != nil {
		selected, err := elem.Property("selected")
		if err == nil {
			isSelected := selected.Bool()
			if isSelected != *sel.Selected {
				return false
			}
		}
	}
	return true
}

// matchesStateFiltersFromAXNode checks state filters using AX node properties.
func (d *Driver) matchesStateFiltersFromAXNode(node *proto.AccessibilityAXNode, sel flow.Selector) bool {
	props := make(map[string]*proto.AccessibilityAXValue)
	for _, p := range node.Properties {
		props[string(p.Name)] = p.Value
	}

	if sel.Enabled != nil {
		if v, ok := props["disabled"]; ok {
			isDisabled := v.Value.Bool()
			if (*sel.Enabled) == isDisabled {
				return false
			}
		}
	}
	if sel.Checked != nil {
		if v, ok := props["checked"]; ok {
			isChecked := v.Value.Str() == "true"
			if *sel.Checked != isChecked {
				return false
			}
		}
	}
	if sel.Focused != nil {
		if v, ok := props["focused"]; ok {
			isFocused := v.Value.Bool()
			if *sel.Focused != isFocused {
				return false
			}
		}
	}
	if sel.Selected != nil {
		if v, ok := props["selected"]; ok {
			isSelected := v.Value.Bool()
			if *sel.Selected != isSelected {
				return false
			}
		}
	}
	return true
}

// elementInfo builds an ElementInfo from a Rod Element.
func (d *Driver) elementInfo(elem *rod.Element) *core.ElementInfo {
	info := &core.ElementInfo{
		Visible: true,
		Enabled: true,
	}

	if text, err := elem.Text(); err == nil {
		info.Text = text
	}

	if shape, err := elem.Shape(); err == nil && shape != nil && len(shape.Quads) > 0 {
		box := shape.Box()
		info.Bounds = core.Bounds{
			X:      int(box.X),
			Y:      int(box.Y),
			Width:  int(box.Width),
			Height: int(box.Height),
		}
	}

	if visible, err := elem.Visible(); err == nil {
		info.Visible = visible
	}

	// Skip attribute lookups for text nodes
	tagName, err := elem.Eval(`() => this.tagName`)
	if err == nil && tagName.Value.Str() != "" {
		info.Class = tagName.Value.Str()

		disabled, err := elem.Property("disabled")
		if err == nil && disabled.Bool() {
			info.Enabled = false
		}

		if checked, err := elem.Property("checked"); err == nil {
			info.Checked = checked.Bool()
		}

		if ariaLabel, err := elem.Attribute("aria-label"); err == nil && ariaLabel != nil {
			info.AccessibilityLabel = *ariaLabel
		}
	}

	return info
}

// calculateTimeout returns the appropriate timeout duration.
func (d *Driver) calculateTimeout(optional bool, stepTimeoutMs int) time.Duration {
	if stepTimeoutMs > 0 {
		return time.Duration(stepTimeoutMs) * time.Millisecond
	}
	if optional {
		return time.Duration(optionalFindTimeoutMs) * time.Millisecond
	}
	return time.Duration(d.findTimeoutMs) * time.Millisecond
}

// cssEscape escapes a string for use in a CSS selector.
func cssEscape(s string) string {
	var b strings.Builder
	for _, c := range s {
		switch {
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_':
			b.WriteRune(c)
		default:
			b.WriteRune('\\')
			b.WriteRune(c)
		}
	}
	return b.String()
}

// looksLikeRegex returns true if the text contains regex metacharacters.
// Matches the behavior of the mobile drivers.
func looksLikeRegex(text string) bool {
	for i := 0; i < len(text); i++ {
		c := text[i]
		if i > 0 && text[i-1] == '\\' {
			continue
		}
		switch c {
		case '.':
			if i+1 < len(text) {
				next := text[i+1]
				if next == '*' || next == '+' || next == '?' {
					return true
				}
			}
		case '*', '+', '?', '[', ']', '{', '}', '|', '(', ')':
			return true
		case '^':
			if i == 0 {
				return true
			}
		case '$':
			if i == len(text)-1 {
				return true
			}
		}
	}
	return false
}

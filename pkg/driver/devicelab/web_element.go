// SPDX-FileCopyrightText: 2024–2026 DeviceLab
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package devicelab

import (
	"fmt"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// WebElement wraps a Rod element to implement core.Element.
type WebElement struct {
	elem *rod.Element
	info *core.ElementInfo
}

func (w *WebElement) Info() *core.ElementInfo { return w.info }

func (w *WebElement) Text() (string, error) { return w.elem.Text() }

func (w *WebElement) Input(text string) error { return w.elem.Input(text) }

// Click performs a CDP click on the element (used for browser mode).
func (w *WebElement) Click() error { return w.elem.Click(proto.InputMouseButtonLeft, 1) }

func (w *WebElement) Clear() error {
	_, err := w.elem.Eval(`() => { this.value = ''; this.dispatchEvent(new Event('input')); }`)
	return err
}

// webElementInfo extracts ElementInfo from a Rod element.
func webElementInfo(elem *rod.Element) *core.ElementInfo {
	info := &core.ElementInfo{
		Visible: true,
		Enabled: true,
	}

	if text, err := elem.Text(); err == nil {
		info.Text = text
	}

	if shape, err := elem.Shape(); err == nil && len(shape.Quads) > 0 {
		box := shape.Box()
		info.Bounds = core.Bounds{
			X:      int(box.X),
			Y:      int(box.Y),
			Width:  int(box.Width),
			Height: int(box.Height),
		}
	}

	// Try to get element tag/type for class
	if tag, err := elem.Eval(`() => this.tagName`); err == nil {
		info.Class = fmt.Sprintf("%v", tag.Value.Val())
	}

	return info
}

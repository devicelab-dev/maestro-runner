// SPDX-FileCopyrightText: 2024 DeviceLab
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package devicelab

import (
	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/uiautomator2"
)

// NativeElement wraps a UIAutomator2 element to implement core.Element.
type NativeElement struct {
	elem   *uiautomator2.Element
	client DeviceLabClient
	info   *core.ElementInfo
}

func (n *NativeElement) Info() *core.ElementInfo { return n.info }

func (n *NativeElement) Text() (string, error) { return n.elem.Text() }

func (n *NativeElement) Input(text string) error { return n.elem.SendKeys(text) }

func (n *NativeElement) Clear() error { return n.elem.Clear() }

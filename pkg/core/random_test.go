// SPDX-FileCopyrightText: 2024 DeviceLab
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package core

import (
	"strings"
	"testing"
)

func TestRandomString(t *testing.T) {
	for _, length := range []int{0, 1, 5, 10, 50, 100} {
		result := RandomString(length)
		if len(result) != length {
			t.Errorf("RandomString(%d) returned length %d", length, len(result))
		}
	}

	// Verify character set
	result := RandomString(200)
	for _, c := range result {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			t.Errorf("RandomString contains invalid character: %c", c)
		}
	}

	// Verify randomness
	r1 := RandomString(20)
	r2 := RandomString(20)
	if r1 == r2 {
		t.Error("RandomString should produce different results on successive calls")
	}
}

func TestRandomEmail(t *testing.T) {
	email := RandomEmail()
	if !strings.Contains(email, "@") {
		t.Errorf("expected @ in email, got: %s", email)
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		t.Errorf("invalid email format: %s", email)
	}
	validDomains := map[string]bool{"example.com": true, "test.com": true, "mail.com": true}
	if !validDomains[parts[1]] {
		t.Errorf("unexpected domain: %s", parts[1])
	}
}

func TestRandomNumber(t *testing.T) {
	for _, length := range []int{0, 1, 5, 10, 20} {
		result := RandomNumber(length)
		if len(result) != length {
			t.Errorf("RandomNumber(%d) returned length %d", length, len(result))
		}
		for _, c := range result {
			if c < '0' || c > '9' {
				t.Errorf("RandomNumber contains non-digit: %c", c)
			}
		}
	}
}

func TestRandomPersonName(t *testing.T) {
	name := RandomPersonName()
	parts := strings.SplitN(name, " ", 2)
	if len(parts) != 2 {
		t.Fatalf("expected 'first last' format, got: %s", name)
	}

	validFirstNames := map[string]bool{
		"John": true, "Jane": true, "Michael": true, "Emily": true, "David": true,
		"Sarah": true, "James": true, "Emma": true, "Robert": true, "Olivia": true,
	}
	validLastNames := map[string]bool{
		"Smith": true, "Johnson": true, "Williams": true, "Brown": true, "Jones": true,
		"Garcia": true, "Miller": true, "Davis": true, "Rodriguez": true, "Martinez": true,
	}

	if !validFirstNames[parts[0]] {
		t.Errorf("unexpected first name: %s", parts[0])
	}
	if !validLastNames[parts[1]] {
		t.Errorf("unexpected last name: %s", parts[1])
	}
}

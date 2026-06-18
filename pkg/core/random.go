// SPDX-FileCopyrightText: 2024–2026 DeviceLab
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package core

import "math/rand"

// RandomString generates a random alphanumeric string of the given length.
func RandomString(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

// RandomEmail generates a random email address.
func RandomEmail() string {
	user := RandomString(8)
	domains := []string{"example.com", "test.com", "mail.com"}
	domain := domains[rand.Intn(len(domains))]
	return user + "@" + domain
}

// RandomNumber generates a random numeric string of the given length.
func RandomNumber(length int) string {
	const digits = "0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = digits[rand.Intn(len(digits))]
	}
	return string(b)
}

// RandomPersonName generates a random full name (first + last).
func RandomPersonName() string {
	firstNames := []string{"John", "Jane", "Michael", "Emily", "David", "Sarah", "James", "Emma", "Robert", "Olivia"}
	lastNames := []string{"Smith", "Johnson", "Williams", "Brown", "Jones", "Garcia", "Miller", "Davis", "Rodriguez", "Martinez"}
	return firstNames[rand.Intn(len(firstNames))] + " " + lastNames[rand.Intn(len(lastNames))]
}

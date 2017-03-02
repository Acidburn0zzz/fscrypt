/*
 * crypto_test.go - tests for the crypto package
 *
 * Copyright 2017 Google Inc.
 * Author: Joe Richey (joerichey@google.com)
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not
 * use this file except in compliance with the License. You may obtain a copy of
 * the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
 * License for the specific language governing permissions and limitations under
 * the License.
 */

package crypto

import (
	"bytes"
	"os"
	"testing"
)

// Reader that always returns the same byte
type ConstReader byte

func (r ConstReader) Read(b []byte) (n int, err error) {
	for i := range b {
		b[i] = byte(r)
	}
	return len(b), nil
}

// Makes a key of the same repeating byte
func makeKey(b byte, n int) (*Key, error) {
	return NewFixedLengthKeyFromReader(ConstReader(b), n)
}

// Tests the two ways of making keys
func TestMakeKeys(t *testing.T) {
	data := []byte("1234\n6789")

	key1, err := NewKeyFromReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, key1.data) {
		t.Error("Key from reader contained incorrect data")
	}

	key2, err := NewFixedLengthKeyFromReader(bytes.NewReader(data), 6)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal([]byte("1234\n6"), key2.data) {
		t.Error("Fixed length key from reader contained incorrect data")
	}
}

// Tests that wipe succeeds
func TestWipe(t *testing.T) {
	key, err := makeKey(1, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if err := key.Wipe(); err != nil {
		t.Error(err)
	}
}

// Making keys with negative length should fail
func TestInvalidLength(t *testing.T) {
	_, err := NewFixedLengthKeyFromReader(bytes.NewReader([]byte{1, 2, 3, 4}), -1)
	if err == nil {
		t.Error("Negative lengths should cause failure")
	}
}

// Test making keys of zero length
func TestZeroLength(t *testing.T) {
	key1, err := NewFixedLengthKeyFromReader(os.Stdin, 0)
	if err != nil {
		t.Fatal(err)
	}
	if key1.data != nil {
		t.Error("FIxed length key from reader contained data")
	}

	key2, err := NewKeyFromReader(bytes.NewReader(nil))
	if err != nil {
		t.Fatal(err)
	}
	if key2.data != nil {
		t.Error("Key from empty reader contained data")
	}
}

// Test making keys long enough that the keys will have to resize
func TestLongLength(t *testing.T) {
	// Key will have to resize 3 times
	data := bytes.Repeat([]byte{1}, os.Getpagesize()*5)
	key, err := NewKeyFromReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, key.data) {
		t.Error("Key contained incorrect data")
	}
}
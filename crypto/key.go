/*
 * key.go - Cryptographic key management for fscrypt. Ensures that sensitive
 * material is properly handled throughout the program.
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
	"io"
	"os"
	"runtime"

	"golang.org/x/sys/unix"

	"fscrypt/util"
)

/*
UseMlock determines whether we should use the mlock/munlock syscalls to
prevent sensitive data like keys and passphrases from being paged to disk.
UseMlock defaults to true, but can be set to false if the application calling
into this library has insufficient privileges to lock memory. A package could
also bind this setting to a flag by using:

	flag.BoolVar(&crypto.UseMlock, "lock-memory", true, "lock keys in memory")
*/
var UseMlock = true

/*
Key protects some arbitrary buffer of cryptographic material. Its methods
ensure that the Key's data is locked in memory before being used (if
UseMlock is set to true), and is wiped and unlocked after use (via the Wipe()
method). This data is never accessed outside of the fscrypt/crypto package
(except for the UnsafeData method). If a key is successfully created, the
Wipe() method should be called after it's use. For example:

	func UseKeyFromStdin() error {
		key, err := NewKeyFromReader(os.Stdin)
		if err != nil {
			return err
		}
		defer key.Wipe()

		// Do stuff with key

		return nil
	}

The Wipe() method will also be called when a key is garbage collected; however,
it is best practice to clear the key as soon as possible, so it spends a minimal
amount of time in memory.

Note that Key is not thread safe, as a key could be wiped while another thread
is using it. Also, calling Wipe() from two threads could cause an error as
memory could be freed twice.
*/
type Key struct {
	data []byte
}

const (
	// Keys need to readable and writable, but hidden from other processes.
	keyProtection = unix.PROT_READ | unix.PROT_WRITE
	keyMmapFlags  = unix.MAP_PRIVATE | unix.MAP_ANONYMOUS
)

// newBlankKey constructs a blank key of a specified length and returns an error
// if we are unable to allocate or lock the necessary memory.
func newBlankKey(length int) (*Key, error) {
	if length == 0 {
		return &Key{data: nil}, nil
	} else if length < 0 {
		return nil, util.InvalidInputF("requested key length %d is negative", length)
	}

	flags := keyMmapFlags
	if UseMlock {
		flags |= unix.MAP_LOCKED
	}

	// See MAP_ANONYMOUS in http://man7.org/linux/man-pages/man2/mmap.2.html
	data, err := unix.Mmap(-1, 0, length, keyProtection, flags)
	if err != nil {
		return nil, util.SystemErrorF("could not mmap() buffer: %v", err)
	}

	key := &Key{data: data}

	// Backup finalizer in case user forgets to "defer key.Wipe()"
	runtime.SetFinalizer(key, (*Key).Wipe)
	return key, nil
}

// Wipe destroys a Key by zeroing and freeing the memory. The data is zeroed
// even if Wipe returns an error, which occurs if we are unable to unlock or
// free the key memory. Calling Wipe() multiple times on a key has no effect.
func (key *Key) Wipe() error {
	if key.data != nil {
		data := key.data
		key.data = nil

		for i := range data {
			data[i] = 0
		}

		if err := unix.Munmap(data); err != nil {
			return util.SystemErrorF("could not munmap() buffer: %v", err)
		}
	}
	return nil
}

// Len is the underlying data buffer's length.
func (key *Key) Len() int {
	return len(key.data)
}

// UnsafeData exposes the underlying protected slice. This is unsafe because the
// data can be paged to disk if the buffer is copied, or the slice may be
// wiped while being used.
func (key *Key) UnsafeData() []byte {
	return key.data
}

// resize returns a new key with size requestedSize and the appropriate data
// copied over. The original data is wiped. This method does nothing and returns
// itself if the key's length equals requestedSize.
func (key *Key) resize(requestedSize int) (*Key, error) {
	if key.Len() == requestedSize {
		return key, nil
	}
	defer key.Wipe()

	resizedKey, err := newBlankKey(requestedSize)
	if err != nil {
		return nil, err
	}
	copy(resizedKey.data, key.data)
	return resizedKey, nil
}

// NewKeyFromReader constructs a key of abritary length by reading from reader
// until hitting EOF.
func NewKeyFromReader(reader io.Reader) (*Key, error) {
	// Use an initial key size of a page. As Mmap allocates a page anyway,
	// there isn't much additional overhead from starting with a whole page.
	key, err := newBlankKey(os.Getpagesize())
	if err != nil {
		return nil, err
	}

	totalBytesRead := 0
	for {
		bytesRead, err := reader.Read(key.data[totalBytesRead:])
		totalBytesRead += bytesRead

		switch err {
		case nil:
			// Need to continue reading. Grow key if necessary
			if key.Len() == totalBytesRead {
				if key, err = key.resize(2 * key.Len()); err != nil {
					return nil, err
				}
			}
		case io.EOF:
			// Getting the EOF error means we are done
			return key.resize(totalBytesRead)
		default:
			// Fail if Read() has a failure
			key.Wipe()
			return nil, err
		}
	}
}

// NewFixedLengthKeyFromReader constructs a key with a specified length by
// reading exactly length bytes from reader.
func NewFixedLengthKeyFromReader(reader io.Reader, length int) (*Key, error) {
	key, err := newBlankKey(length)
	if err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(reader, key.data); err != nil {
		key.Wipe()
		return nil, err
	}
	return key, nil
}
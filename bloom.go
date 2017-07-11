// DCSO go bloom filter
// Copyright (c) 2017, DCSO GmbH

//Implements a simple and highly efficient variant of the Bloom filter that uses only two hash functions.

package bloom

import "encoding/binary"
import "hash/fnv"
import "errors"
import "math"
import "fmt"
import "io"

const magicSeed = "this-is-magical"

// SetError represents an error with a given message related to set operations.
type SetError struct {
	msg string
}

// BloomFilter represents a Bloom filter, a data structure for quickly checking
// for set membership, with a specific desired capacity and false positive
// probability.
type BloomFilter struct {
	//bit array
	v []uint64
	//desired maximum number of elements
	n uint32
	//desired false positive probability
	p float64
	//number of hash functions
	k uint32
	//number of bits
	m uint32
	//number of elements in the filter
	N uint32

	//number of 64-bit integers (generated automatically)
	M uint32
}

// Read loads a filter from a reader object.
func (s *BloomFilter) Read(input io.Reader) error {
	bs4 := make([]byte, 4)
	bs8 := make([]byte, 8)

	if _, err := io.ReadFull(input, bs4); err != nil {
		return err
	}

	s.n = binary.LittleEndian.Uint32(bs4)

	if _, err := io.ReadFull(input, bs8); err != nil {
		return err
	}

	s.p = math.Float64frombits(binary.LittleEndian.Uint64(bs8))

	if _, err := io.ReadFull(input, bs4); err != nil {
		return err
	}

	s.k = binary.LittleEndian.Uint32(bs4)

	if _, err := io.ReadFull(input, bs4); err != nil {
		return err
	}

	s.m = binary.LittleEndian.Uint32(bs4)

	if _, err := io.ReadFull(input, bs4); err != nil {
		return err
	}

	s.N = binary.LittleEndian.Uint32(bs4)

	s.M = uint32(math.Ceil(float64(s.m) / 64.0))

	s.v = make([]uint64, s.M)

	for i := uint32(0); i < s.M; i++ {
		n, err := io.ReadFull(input, bs8)
		if err != nil {
			return err
		}
		if n != 8 {
			return fmt.Errorf("Cannot read from file: %d, position: %d, %d", n, i*8, len(bs8))
		}
		s.v[i] = binary.LittleEndian.Uint64(bs8)
	}

	return nil

}

// NumHashFuncs returns the number of hash functions used in the Bloom filter.
func (s *BloomFilter) NumHashFuncs() uint32 {
	return s.k
}

// MaxNumElements returns the maximal supported number of elements in the Bloom
// filter (capacity).
func (s *BloomFilter) MaxNumElements() uint32 {
	return s.n
}

// NumBits returns the number of bits used in the Bloom filter.
func (s *BloomFilter) NumBits() uint32 {
	return s.m
}

// FalsePositiveProb returns the chosen false positive probability for the
// Bloom filter.
func (s *BloomFilter) FalsePositiveProb() float64 {
	return s.p
}

// Write writes the binary representation of a Bloom filter to an io.Writer.
func (s *BloomFilter) Write(output io.Writer) error {
	bs4 := make([]byte, 4)
	bs8 := make([]byte, 8)

	binary.LittleEndian.PutUint32(bs4, s.n)
	output.Write(bs4)
	binary.LittleEndian.PutUint64(bs8, math.Float64bits(s.p))
	output.Write(bs8)
	binary.LittleEndian.PutUint32(bs4, s.k)
	output.Write(bs4)
	binary.LittleEndian.PutUint32(bs4, s.m)
	output.Write(bs4)
	binary.LittleEndian.PutUint32(bs4, s.N)
	output.Write(bs4)

	for i := uint32(0); i < s.M; i++ {
		binary.LittleEndian.PutUint64(bs8, s.v[i])
		n, err := output.Write(bs8)
		if n != 8 {
			return errors.New("Cannot write to file!")
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Reset clears the Bloom filter of all elements.
func (s *BloomFilter) Reset() {
	for i := uint32(0); i < s.M; i++ {
		s.v[i] = 0
	}
	s.N = 0
}

// Fingerprint returns the fingerprint of a given value, as an array of index
// values.
func (s *BloomFilter) Fingerprint(value []byte, fingerprint []uint32) {

	hashValue1 := fnv.New64()
	hashValue2 := fnv.New64()

	hashValue1.Write(value)
	hashValue2.Write(value)
	hashValue2.Write([]byte(magicSeed))

	h1 := hashValue1.Sum64()
	h2 := hashValue2.Sum64()

	for i := uint32(0); i < s.k; i++ {
		fingerprint[i] = uint32((h1 + (uint64(i)+1)*h2) % uint64(s.m))
	}
}

// Add adds a byte array element to the Bloom filter.
func (s *BloomFilter) Add(value []byte) {
	var k, l uint32
	newValue := false
	fingerprint := make([]uint32, s.k)
	s.Fingerprint(value, fingerprint)
	for i := uint32(0); i < s.k; i++ {
		k = uint32(fingerprint[i] / 64)
		l = uint32(fingerprint[i] % 64)
		v := uint64(1 << l)
		if s.v[k]&v == 0 {
			newValue = true
		}
		s.v[k] |= v
	}
	if newValue {
		s.N++
	}
}

// Check returns true if the given value may be in the Bloom filter, false if it
// is definitely not in it.
func (s *BloomFilter) Check(value []byte) bool {
	fingerprint := make([]uint32, s.k)
	s.Fingerprint(value, fingerprint)
	return s.CheckFingerprint(fingerprint)
}

// CheckFingerprint returns true if the given fingerprint occurs in the Bloom
// filter, false if it does not.
func (s *BloomFilter) CheckFingerprint(fingerprint []uint32) bool {
	var k, l uint32
	for i := uint32(0); i < s.k; i++ {
		k = uint32(fingerprint[i] / 64)
		l = uint32(fingerprint[i] % 64)
		if (s.v[k] & (1 << l)) == 0 {
			return false
		}
	}
	return true
}

// Initialize returns a new, empty Bloom filter with the given capacity (n)
// and FP probability (p).
func Initialize(n uint32, p float64) BloomFilter {
	var bf BloomFilter
	bf.n = n
	bf.p = p
	bf.m = uint32(math.Abs(math.Ceil(float64(n) * math.Log(float64(p)) / (math.Pow(math.Log(2.0), 2.0)))))
	bf.M = uint32(math.Ceil(float64(bf.m) / 64.0))
	bf.k = uint32(math.Ceil(math.Log(2) * float64(bf.m) / float64(n)))
	bf.v = make([]uint64, bf.M)
	return bf
}

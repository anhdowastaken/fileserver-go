package utilities

import (
	"crypto/md5"
	"encoding/hex"
	"regexp"
)

// BytesToMD5Bytes returns MD5 hash bytes of a byte array
func BytesToMD5Bytes(array []byte) []byte {
	algorithm := md5.New()
	algorithm.Write(array)
	return algorithm.Sum(nil)
}

// StringToMD5Bytes returns MD5 hash bytes of a string
func StringToMD5Bytes(text string) []byte {
	algorithm := md5.New()
	algorithm.Write([]byte(text))
	return algorithm.Sum(nil)
}

// BytesToMD5String returns MD5 hash string of a byte array
func BytesToMD5String(array []byte) string {
	algorithm := md5.New()
	algorithm.Write(array)
	return hex.EncodeToString(algorithm.Sum(nil))
}

// StringToMD5String return MD5 hash string of a string
func StringToMD5String(text string) string {
	algorithm := md5.New()
	algorithm.Write([]byte(text))
	return hex.EncodeToString(algorithm.Sum(nil))
}

func SanitizeFilename(filename string) string {
	// https://en.wikipedia.org/wiki/Filename#Reserved_characters_and_words
	rep := regexp.MustCompile(`[\x5C\x2F\x3F\x25\x2A\x3A\x7C\x22\x3E\x3C\x20]`)

	return rep.ReplaceAllString(filename, "_")
}

package ziputil

import (
	"archive/zip"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func DecodeName(f *zip.File) string {
	name := f.Name
	if utf8.ValidString(name) {
		return name
	}
	for _, dec := range decoders {
		decoded, _, err := transform.String(dec, name)
		if err == nil && utf8.ValidString(decoded) && looksLikeText(decoded) {
			return decoded
		}
	}
	return name
}

var decoders = []transform.Transformer{
	simplifiedchinese.GBK.NewDecoder(),
	japanese.ShiftJIS.NewDecoder(),
}

func looksLikeText(s string) bool {
	nonAscii := 0
	total := 0
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			nonAscii++
			total++
		} else if r >= 0x3400 && r <= 0x4DBF {
			nonAscii++
			total++
		} else if r >= 0x3040 && r <= 0x30FF {
			nonAscii++
			total++
		} else if r >= 0xFF00 && r <= 0xFFEF {
			nonAscii++
			total++
		} else if r > 127 {
			total++
		}
	}
	if total == 0 {
		return true
	}
	return float64(nonAscii)/float64(total) > 0.15 || nonAscii >= 3
}

// Reproducer: lz4 v2.6.1 amd64 assembly decoder fails on a valid N3 P2P payload.
//
//	go run main.go            # on linux/amd64 -> FAILS (asm decoder)
//	GOARCH=arm64 go run main.go  # or any non-amd64 -> works (pure-Go decoder)
//	go run -tags noasm main.go   # on linux/amd64 -> works (pure-Go decoder)
//
// Both .bin files carry the same 6981-byte N3 message, compressed independently
// (one by the C# node's K4os LZ4, one by neo-go's own lz4 v2.6.1 CompressBlock):
// 4-byte little-endian decompressed length, then the raw LZ4 block stream.
package main

import (
	"encoding/binary"
	"fmt"
	"os"

	"github.com/pierrec/lz4"
)

func main() {
	for _, fn := range []string{"payload-neo-csharp.bin", "payload-neo-go.bin"} {
		d, err := os.ReadFile(fn)
		if err != nil {
			fmt.Println("read:", err)
			os.Exit(1)
		}
		want := binary.LittleEndian.Uint32(d[:4])
		dst := make([]byte, want)
		n, err := lz4.UncompressBlock(d[4:], dst)
		status := "OK"
		if err != nil || uint32(n) != want {
			status = "FAIL"
		}
		fmt.Printf("%-22s want=%d got=%d err=%v  -> %s\n", fn, want, n, err, status)
	}
}

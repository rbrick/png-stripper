package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	inputDirectory  = flag.String("input", "images", "The path to the PNGs that need to be fixed")
	outputDirectory = flag.String("output", "processed", "The path to the output directory")
	// Used for checking passed in images
	checkFlag = flag.Bool("check", false, "run with this flag if you just want to check for broken PNGs")
	// compress w/ webp
	webpFlag = flag.Bool("compress", false, "compress the stripped down image with webp")

	routinesFlag = flag.Int("routines", 16, "the amount of go routines to spawn")
)

func init() {
	flag.Parse() // our flags

	log.Printf("input directory: %s, output directory: %s, goroutine count: %d\ncompress to webp: %t, integrity check: %t",
		*inputDirectory, *outputDirectory, *routinesFlag, *webpFlag, *checkFlag)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func strip(png *PNG, output string, compress, check bool) error {
	var byteBuf bytes.Buffer

	byteBuf.Write(PNGHeader)

	png.Chunks["IHDR"][0].Write(&byteBuf)

	for _, chunks := range png.Chunks {
		for _, chunk := range chunks {
			// throw away all ancillary chunks
			if chunk.Type == "IDAT" || chunk.Type == "PLTE" {

				if check {
					_, err := chunk.Verify()
					if err != nil {
						// failed a checksum
						return errors.New(fmt.Sprintf("%s failed checksum", output))
					}
				}

				chunk.Write(&byteBuf)
			}
		}
	}

	png.Chunks["IEND"][0].Write(&byteBuf)

	if compress {
		output = output[:strings.LastIndex(output, ".")] + ".webp"

		temp, err := ioutil.TempFile("", "strip-*.png")

		if err != nil {
			return err
		}

		temp.Write(byteBuf.Bytes())
		temp.Close()

		cmd := exec.Command("cwebp", "-lossless", temp.Name(), "-o", output)

		if err = cmd.Start(); err != nil {
			return err
		}
	} else {
		f, err := os.Create(output)

		if err != nil {
			return err
		}

		f.Write(byteBuf.Bytes())
		f.Close()
	}
	return nil
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	var waitGroup sync.WaitGroup

	start := time.Now()

	files, _ := ioutil.ReadDir(*inputDirectory)

	tasks := make(chan func() error, len(files))

	filepath.Walk(*inputDirectory, func(path string, info os.FileInfo, err error) error {
		if strings.HasSuffix(info.Name(), ".png") {
			f, _ := os.Open(path)
			tasks <- func() error {
				png, err := Read(f)

				if err != nil {
					if err == ErrorCRCMismatch {
						fmt.Printf("crc mismatch while reading %s\n", path)
					}
					return err
				}

				p := *outputDirectory + path[strings.LastIndex(path, string(os.PathSeparator)):]

				return strip(png, p, *webpFlag, *checkFlag)
			}
		}

		return err
	})

	close(tasks)

	end := time.Now()

	log.Println("collected tasks, took", end.Sub(start).Seconds(), "seconds")

	for i := 0; i < *routinesFlag; i++ {
		waitGroup.Add(1)
		fmt.Printf("starting work group %d\n", i)
		taskID := i
		go func() {

			for f := range tasks {
				e := f()

				if e != nil {
					log.Println(e)
				}
			}

			fmt.Printf("worker group %d completed\n", taskID)
			waitGroup.Done()
		}()
	}

	start = time.Now()

	waitGroup.Wait()
	end = time.Now()
	fmt.Println("completed in", end.Sub(start).Seconds(), "seconds")
}

package main

import (
	"flag"
	"log"
	"os"
	"time"

	"github.com/go-audio/wav"
	"github.com/gordonklaus/portaudio"

	"gopkg.in/hraban/opus.v2"

	"bytes"
	"encoding/binary"
	"fmt"
	"net"
)

const (
	ServerAddr = "localhost:8081"
	// ServerAddr = "speccy49home.ddns.net:8081"
)

const SIGNATURE = 0x4C2AE6CC

type MessageType byte

const (
	AUDIO MessageType = 0x0
)

type Packet struct {
	Signature   uint32
	MessageType MessageType
	StreamID    uint32
	FrameNumber uint32
	Timestamp   uint64
	SampleRate  uint32
	FrameLength uint32
	Frame       []byte
}




func (p *Packet) Encode() []byte {
	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.BigEndian, p.Signature)
	_ = binary.Write(buf, binary.BigEndian, p.MessageType)
	_ = binary.Write(buf, binary.BigEndian, p.StreamID)
	_ = binary.Write(buf, binary.BigEndian, p.FrameNumber)
	_ = binary.Write(buf, binary.BigEndian, p.Timestamp)
	_ = binary.Write(buf, binary.BigEndian, p.SampleRate)
	_ = binary.Write(buf, binary.BigEndian, p.FrameLength)
	for _, sample := range p.Frame {
		_ = binary.Write(buf, binary.BigEndian, sample)
	}
	return buf.Bytes()
}

func Decode(data []byte) Packet {
	buf := bytes.NewBuffer(data)
	var p Packet
	_ = binary.Read(buf, binary.BigEndian, &p.Signature)
	_ = binary.Read(buf, binary.BigEndian, &p.MessageType)
	_ = binary.Read(buf, binary.BigEndian, &p.StreamID)
	_ = binary.Read(buf, binary.BigEndian, &p.FrameNumber)
	_ = binary.Read(buf, binary.BigEndian, &p.Timestamp)
	_ = binary.Read(buf, binary.BigEndian, &p.SampleRate)
	_ = binary.Read(buf, binary.BigEndian, &p.FrameLength)
	p.Frame = make([]byte, p.FrameLength)
	for i := 0; i < int(p.FrameLength); i++ {
		_ = binary.Read(buf, binary.BigEndian, &p.Frame[i])
	}
	return p
}



func main() {
	filePath := flag.String("file", "", "Path to the audio file")
	streamID := flag.Int("streamid", -1, "Stream ID (required)")
	flag.Parse()

	if *streamID == -1 {
		log.Fatal("Error: -streamid flag is required")
	}

	if *filePath != "" {
		processFile(*filePath, *streamID)
	} else {
		recordFromMic(*streamID)
	}
}

func processFile(filePath string, streamID int) {
	file, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	decoder := wav.NewDecoder(file)
	audioBuf, err := decoder.FullPCMBuffer()
	if err != nil {
		log.Fatalf("Failed to decode WAV file: %v", err)
	}

	sampleRate := decoder.SampleRate
	opusEncoder, err := opus.NewEncoder(int(sampleRate), 1, opus.AppAudio)
	if err != nil {
		log.Fatal(err)
	}

	addr, err := net.ResolveUDPAddr("udp", ServerAddr)
	if err != nil {
		log.Fatalf("Failed to resolve UDP address: %v", err)
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Fatalf("Failed to connect to UDP server: %v", err)
	}
	defer conn.Close()

	var frameNumber uint32 = 0
	frameSize := 960 // Adjust based on your encoding settings

	for i := 0; i < len(audioBuf.Data); i += frameSize {
		
		
		end := i + frameSize
		if end > len(audioBuf.Data) {
			end = len(audioBuf.Data)
		}
		frame := audioBuf.Data[i:end]
		encoded := make([]byte, 1024)

		n, err := opusEncoder.Encode(int16Slice(frame), encoded)
		if err != nil {
			log.Printf("Could not encode sound with OPUS: %v", err)
			continue
		}
		fmt.Println(n)

		packet := Packet{
			Signature:   SIGNATURE,
			StreamID:    uint32(streamID),
			MessageType: AUDIO,
			FrameNumber: frameNumber,
			Timestamp:   uint64(time.Now().UnixMicro()),
			SampleRate:  uint32(sampleRate),
			FrameLength: uint32(n),
			Frame:       encoded[:n],
		}

		_, err = conn.Write(packet.Encode())
		if err != nil {
			log.Printf("Failed to send packet: %v", err)
		}

		frameNumber++
	}
}


func int16Slice(data []int) []int16 {
	out := make([]int16, len(data))
	for i, d := range data {
		out[i] = int16(d)
	}
	return out
}

func recordFromMic(streamID int) {
	err := portaudio.Initialize()
	if err != nil {
		log.Fatalf("Failed to initialize PortAudio: %v", err)
	}
	defer portaudio.Terminate()

	sampleRate := 48000
	bufferSize := 960
	in := make([]int16, bufferSize)
	opusEncoder, err := opus.NewEncoder(sampleRate, 1, opus.AppAudio)
	if err != nil {
		log.Fatal(err)
	}

	stream, err := portaudio.OpenDefaultStream(1, 0, float64(sampleRate), len(in), in)
	if err != nil {
		log.Fatalf("Failed to open audio stream: %v", err)
	}
	defer stream.Close()

	err = stream.Start()
	if err != nil {
		log.Fatalf("Failed to start audio stream: %v", err)
	}
	fmt.Println("Recording...")

	addr, err := net.ResolveUDPAddr("udp", ServerAddr)
	if err != nil {
		log.Fatalf("Failed to resolve UDP address: %v", err)
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Fatalf("Failed to connect to UDP server: %v", err)
	}
	defer conn.Close()

	start := time.Now()
	var frameNumber uint32 = 0

	for time.Since(start) < 10*60*time.Second {
		if err := stream.Read(); err != nil {
			log.Fatalf("Failed to read from stream: %v", err)
		}

		encoded := make([]byte, 1024)
		n, err := opusEncoder.Encode(in, encoded)
		if err != nil {
			log.Printf("Could not encode sound with OPUS: %v", err)
			continue
		}

		packet := Packet{
			Signature:   SIGNATURE,
			StreamID:    uint32(streamID),
			MessageType: AUDIO,
			FrameNumber: frameNumber,
			Timestamp:   uint64(time.Now().UnixNano()),
			SampleRate:  uint32(sampleRate),
			FrameLength: uint32(n),
			Frame:       encoded[:n],
		}

		_, err = conn.Write(packet.Encode())
		if err != nil {
			log.Printf("Failed to send packet: %v", err)
		}

		frameNumber++
	}

	err = stream.Stop()
	if err != nil {
		log.Fatalf("Failed to stop audio stream: %v", err)
	}

	log.Println("Recording finished.")
}

package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
	"github.com/gordonklaus/portaudio"

	"gopkg.in/hraban/opus.v2"

	"bytes"
	"encoding/binary"
	"fmt"
	"net"
)

const (
	// ServerAddr = "localhost:8081"
	ServerAddr = "speccy49home.ddns.net:8081"
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

	StreamId, err:= strconv.Atoi(os.Args[1])
	if err != nil {
		panic("AAAAAA")
	}

	err = portaudio.Initialize()
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

	start := time.Now()
	var i uint32 = 0

	outFile, err := os.Create("output.wav")
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer outFile.Close()

	encoder := wav.NewEncoder(outFile, sampleRate, 16, 1, 1)
	buf := &audio.IntBuffer{Data: make([]int, 0, sampleRate), Format: &audio.Format{SampleRate: sampleRate, NumChannels: 1}}

	addr, err := net.ResolveUDPAddr("udp", ServerAddr)
	log.Printf("UDP: %v", err)
	conn, err := net.DialUDP("udp", nil, addr)
	log.Printf("UDP: %v", err)
	defer conn.Close()

	for time.Since(start) < 60*time.Second {
		if err := stream.Read(); err != nil {
			log.Fatalf("Failed to read from stream: %v", err)
		}

		for _, sample := range in {
			buf.Data = append(buf.Data, int(sample))
		}

		// Copy data to buffer
		encoded := make([]byte, 1024)
		n, err := opusEncoder.Encode(in, encoded)
		if err != nil {
			log.Printf("Could not encode sound withj OPUS: %s", err)
		}
		packet := Packet{
			Signature:   SIGNATURE,
			StreamID:    uint32(StreamId),
			MessageType: AUDIO,
			FrameNumber: i,
			Timestamp:   uint64(time.Now().Unix()),
			SampleRate:  uint32(sampleRate),
			FrameLength: uint32(n),
			Frame:       encoded[:n],
		}

		_, err = conn.Write(packet.Encode())
		if err != nil {
			log.Println(err)
		}
		i += 1

	}

	err = stream.Stop()
	if err != nil {
		log.Fatalf("Failed to stop audio stream: %v", err)
	}

	if err := encoder.Write(buf); err != nil {
		log.Fatalf("Failed to write data to WAV file: %v", err)
	}

	// Finalize and close encoder
	if err := encoder.Close(); err != nil {
		log.Fatalf("Failed to close WAV encoder: %v", err)
	}

	log.Println("Recording saved to output.wav")

}

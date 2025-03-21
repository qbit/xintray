package main

import (
	"bytes"
	"encoding/binary"
	"net"
)

type macaddr [6]byte

type magicPacket struct {
	header [6]byte
	load   [16]macaddr
}

func sendMagicPacket(mac net.HardwareAddr) error {
	var pkt magicPacket
	var maca macaddr

	for i := range mac {
		maca[i] = mac[i]
	}

	// FFFFFFFFFFF
	for i := range pkt.header {
		pkt.header[i] = 0xFF
	}

	// MACMACMACMACMACMAC
	for i := range pkt.load {
		pkt.load[i] = maca
	}

	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.BigEndian, pkt); err != nil {
		return err
	}

	broadcastAddr, err := net.ResolveUDPAddr("udp", "255.255.255.255:7")
	if err != nil {
		return err
	}

	conn, err := net.DialUDP("udp", nil, broadcastAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Write(buf.Bytes())
	if err != nil {
		return err
	}
	return nil
}

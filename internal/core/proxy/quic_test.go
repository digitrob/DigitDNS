package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
)

func testQUICInitial(dcid, scid []byte) []byte {
	pkt := make([]byte, 0, quicMinInitialPacket)
	pkt = append(pkt, 0xc0) // long header, fixed bit, Initial
	pkt = binary.BigEndian.AppendUint32(pkt, 1)
	pkt = append(pkt, byte(len(dcid)))
	pkt = append(pkt, dcid...)
	pkt = append(pkt, byte(len(scid)))
	pkt = append(pkt, scid...)
	// The client pads an Initial datagram to at least 1200 bytes.
	pkt = append(pkt, make([]byte, quicMinInitialPacket-len(pkt))...)
	return pkt
}

func TestBuildVersionNegotiationSwapsConnectionIDs(t *testing.T) {
	dcid := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	scid := []byte{9, 10, 11, 12}
	out := buildVersionNegotiation(dcid, scid)

	if out[0]&0x80 == 0 {
		t.Fatal("reply is not a long-header packet")
	}
	if v := binary.BigEndian.Uint32(out[1:5]); v != 0 {
		t.Fatalf("version = %d, want 0 (Version Negotiation)", v)
	}
	pos := 5
	if got := int(out[pos]); got != len(scid) {
		t.Fatalf("destination CID length = %d, want %d", got, len(scid))
	}
	pos++
	if !bytes.Equal(out[pos:pos+len(scid)], scid) {
		t.Fatal("destination CID was not the client's source CID")
	}
	pos += len(scid)
	if got := int(out[pos]); got != len(dcid) {
		t.Fatalf("source CID length = %d, want %d", got, len(dcid))
	}
	pos++
	if !bytes.Equal(out[pos:pos+len(dcid)], dcid) {
		t.Fatal("source CID was not the client's destination CID")
	}
	pos += len(dcid)
	if got := binary.BigEndian.Uint32(out[pos:]); got != quicGreasedVersion {
		t.Fatalf("offered version = %#x, want the reserved greased version", got)
	}
}

func TestParseQUICLongHeaderRejectsOtherTraffic(t *testing.T) {
	short := make([]byte, 1200)
	short[0] = 0x40 // short header: not a handshake start
	if _, _, ok := parseQUICLongHeader(short); ok {
		t.Error("short header parsed as a long header")
	}

	vn := testQUICInitial([]byte{1}, []byte{2})
	binary.BigEndian.PutUint32(vn[1:5], 0)
	if _, _, ok := parseQUICLongHeader(vn); ok {
		t.Error("a Version Negotiation packet was answered with another one")
	}

	if _, _, ok := parseQUICLongHeader(make([]byte, 6)); ok {
		t.Error("a truncated packet parsed as a long header")
	}
}

func TestQUICRejectorAnswersAnInitial(t *testing.T) {
	server, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	rejector := newQUICRejector(server)
	go rejector.run(context.Background())

	client, err := net.DialUDP("udp", nil, server.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	dcid := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	scid := []byte{9, 10, 11, 12}
	if _, err := client.Write(testQUICInitial(dcid, scid)); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1500)
	n, err := client.Read(buf)
	if err != nil {
		t.Fatalf("no Version Negotiation reply: %v", err)
	}
	reply := buf[:n]
	if binary.BigEndian.Uint32(reply[1:5]) != 0 {
		t.Fatal("reply is not a Version Negotiation packet")
	}
	if got := int(reply[5]); got != len(scid) {
		t.Fatalf("reply destination CID length = %d, want %d", got, len(scid))
	}
	if rejected, _, _ := rejector.stats(); rejected != 1 {
		t.Fatalf("rejected = %d, want 1", rejected)
	}
}

func TestQUICRejectorMakesARealClientAbortFast(t *testing.T) {
	server, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	rejector := newQUICRejector(server)
	go rejector.run(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	conn, err := quic.DialAddr(ctx, server.LocalAddr().String(), &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"h3"},
	}, nil)
	elapsed := time.Since(start)
	if err == nil {
		_ = conn.CloseWithError(0, "")
		t.Fatal("QUIC handshake against the TCP-only relay unexpectedly succeeded")
	}
	// A real client that receives the Version Negotiation packet aborts in a
	// round trip. Without the guard it would sit through its own handshake
	// timeout (seconds), which is the delay this feature removes.
	if elapsed > 2*time.Second {
		t.Fatalf("client waited %v for the handshake to fail; the guard did not fail fast", elapsed)
	}
	if rejected, _, _ := rejector.stats(); rejected == 0 {
		t.Fatal("the client aborted but the guard counted no reply")
	}
}

func TestQUICRejectorIgnoresSmallPackets(t *testing.T) {
	server, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	rejector := newQUICRejector(server)
	go rejector.run(context.Background())

	client, err := net.DialUDP("udp", nil, server.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// A small long-header packet is below the Initial minimum and must not be
	// answered: that is what keeps the guard from reflecting spoofed probes.
	if _, err := client.Write([]byte{0xc0, 0, 0, 0, 1, 0, 0}); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if n, _, err := client.ReadFromUDP(make([]byte, 1500)); err == nil {
		t.Fatalf("small datagram was answered with %d bytes", n)
	}
	if rejected, ignored, _ := rejector.stats(); ignored != 1 || rejected != 0 {
		t.Fatalf("stats = ignored %d / rejected %d, want 1/0", ignored, rejected)
	}
}

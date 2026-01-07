package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	opuscodec "github.com/jj11hh/opus"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

const DefaultModel = "gpt-4o-realtime-preview"

// Sentinel errors.
var (
	ErrNotReady = errors.New("client not ready")
	ErrClosed   = errors.New("client closed")
)

// Client handles WebRTC connection to OpenAI Realtime API.
//
// Memory layout optimized: hot path fields grouped together,
// cold path fields at the end.
type Client struct {
	// ─── Hot path (audio encoding) ───────────────────────────────────────────
	opusEncoder *opuscodec.Encoder             // 8 bytes
	audioTrack  *webrtc.TrackLocalStaticSample // 8 bytes
	opusBuffer  []byte                         // slice header 24 bytes

	// ─── Synchronization ─────────────────────────────────────────────────────
	mu     sync.Mutex // protects closed flag and initialization
	closed bool

	// ─── Cold path (connection state) ────────────────────────────────────────
	apiKey            string
	sessionCfg        SessionConfig
	peerConnection    *webrtc.PeerConnection
	dataChannel       *webrtc.DataChannel
	msgChan           chan Event
	errChan           chan error
	done              chan struct{}
	onDataChannelOpen func()
}

// Config holds configuration for the client.
type Config struct {
	APIKey  string
	Session SessionConfig // Transcription session config
}

// NewClient creates a new WebRTC-based Realtime client.
func NewClient(cfg Config) (*Client, error) {
	return &Client{
		apiKey:     cfg.APIKey,
		sessionCfg: cfg.Session,
		msgChan:    make(chan Event, 100),
		errChan:    make(chan error, 1),
		done:       make(chan struct{}),
		// Max Opus packet size is typically 1275 bytes
		opusBuffer: make([]byte, 1275),
	}, nil
}

// OnDataChannelOpen sets a callback to be called when the data channel opens.
func (c *Client) OnDataChannelOpen(callback func()) {
	c.mu.Lock()
	c.onDataChannelOpen = callback
	c.mu.Unlock()
}

// Connect establishes WebRTC connection to OpenAI Realtime API.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClosed
	}
	c.mu.Unlock()

	// Step 1: Create ephemeral transcription session
	slog.Info("creating OpenAI realtime transcription session")
	sessionToken, err := CreateSession(ctx, c.apiKey, c.sessionCfg)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	slog.Info("session created", "expires", time.Unix(sessionToken.ExpiresAt, 0))

	// Step 2: Create peer connection
	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return fmt.Errorf("register codecs: %w", err)
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))
	pc, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}},
	})
	if err != nil {
		return fmt.Errorf("create peer connection: %w", err)
	}

	// Step 3: Audio track setup
	audioTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeOpus,
			ClockRate: 48000,
			Channels:  2,
		},
		"audio",
		"transy-audio",
	)
	if err != nil {
		pc.Close()
		return fmt.Errorf("create audio track: %w", err)
	}

	if _, err = pc.AddTrack(audioTrack); err != nil {
		pc.Close()
		return fmt.Errorf("add audio track: %w", err)
	}

	// Initialize Opus encoder (48kHz stereo)
	opusEnc, err := opuscodec.NewEncoder(48000, 2, opuscodec.AppRestrictedLowdelay)
	if err != nil {
		pc.Close()
		return fmt.Errorf("create opus encoder: %w", err)
	}

	// Step 4: Data channel
	dc, err := pc.CreateDataChannel("oai-events", nil)
	if err != nil {
		pc.Close()
		return fmt.Errorf("create data channel: %w", err)
	}

	// Store state under lock
	c.mu.Lock()
	c.peerConnection = pc
	c.audioTrack = audioTrack
	c.opusEncoder = opusEnc
	c.dataChannel = dc
	c.mu.Unlock()

	dc.OnOpen(func() {
		slog.Info("data channel opened")
		c.mu.Lock()
		callback := c.onDataChannelOpen
		c.mu.Unlock()
		if callback != nil {
			go callback()
		}
	})

	dc.OnMessage(c.handleDataMessage)

	// Step 5: Remote track handler (ignore incoming audio)
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		go func() {
			buf := make([]byte, 1500)
			for {
				if _, _, err := track.Read(buf); err != nil {
					return
				}
			}
		}()
	})

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if state == webrtc.ICEConnectionStateFailed || state == webrtc.ICEConnectionStateClosed {
			select {
			case c.errChan <- fmt.Errorf("ICE connection %s", state.String()):
			default:
			}
		}
	})

	// Step 6: SDP Exchange
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("create offer: %w", err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("set local description: %w", err)
	}

	<-webrtc.GatheringCompletePromise(pc)

	answerSDP, err := ExchangeSDP(ctx, pc.LocalDescription().SDP, sessionToken.Value)
	if err != nil {
		return fmt.Errorf("exchange SDP: %w", err)
	}

	if err := pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answerSDP,
	}); err != nil {
		return fmt.Errorf("set remote description: %w", err)
	}

	return nil
}

func (c *Client) handleDataMessage(msg webrtc.DataChannelMessage) {
	slog.Debug("on message", "data", string(msg.Data))
	event, err := ParseEvent(msg.Data)
	if err != nil {
		slog.Warn("failed to parse event", "error", err)
		return
	}

	select {
	case c.msgChan <- event:
	case <-time.After(50 * time.Millisecond):
		slog.Warn("msg channel full", "type", event.eventType())
	}
}

// SendAudio encodes and sends audio samples.
//
// Expects stereo interleaved float32 samples at 48kHz.
func (c *Client) SendAudio(samples []float32) error {
	// Snapshot references under lock
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClosed
	}
	track := c.audioTrack
	encoder := c.opusEncoder
	c.mu.Unlock()

	if track == nil || encoder == nil {
		return ErrNotReady
	}

	// Encode directly - samples are already stereo
	n, err := encoder.EncodeFloat32(samples, c.opusBuffer)
	if err != nil {
		return fmt.Errorf("opus encode: %w", err)
	}

	// WriteSample copies the data internally
	// Duration = samples / 2 (stereo) / 48000
	sample := media.Sample{
		Data:     c.opusBuffer[:n],
		Duration: time.Duration(len(samples)/2) * time.Second / 48000,
	}

	return track.WriteSample(sample)
}

// Messages returns the channel for receiving parsed events.
func (c *Client) Messages() <-chan Event {
	return c.msgChan
}

// Errors returns the channel for receiving connection errors.
func (c *Client) Errors() <-chan error {
	return c.errChan
}

// Close shuts down the client and releases resources.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true
	close(c.done)

	if c.peerConnection != nil {
		_ = c.peerConnection.Close()
	}
	close(c.msgChan)
	return nil
}

// ConfigureVAD sends a session.update to configure voice activity detection.
func (c *Client) ConfigureVAD(td TurnDetection) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClosed
	}
	dc := c.dataChannel
	c.mu.Unlock()

	if dc == nil || dc.ReadyState() != webrtc.DataChannelStateOpen {
		return ErrNotReady
	}

	msg := SessionUpdate{Type: "session.update"}
	msg.Session.TurnDetection = &td

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal session update: %w", err)
	}

	slog.Debug("sending session.update", "turn_detection", td)
	return dc.SendText(string(data))
}

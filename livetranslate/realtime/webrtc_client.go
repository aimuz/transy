package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	opuscodec "github.com/jj11hh/opus"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

const (
	DefaultModel = "gpt-4o-realtime-preview-2024-12-17"
)

// Client handles WebRTC connection to OpenAI Realtime API.
// This provides lower latency than WebSocket for real-time audio streaming.
type Client struct {
	apiKey string
	model  string

	// Session management
	sessionMgr *SessionManager

	// WebRTC components
	peerConnection *webrtc.PeerConnection
	dataChannel    *webrtc.DataChannel
	audioTrack     *webrtc.TrackLocalStaticSample
	opusEncoder    *opuscodec.Encoder

	// Event channels
	msgChan chan ClientEvent
	errChan chan error
	done    chan struct{}

	// Callbacks
	onDataChannelOpen func()

	// State
	mu     sync.Mutex
	closed bool
}

// Config holds configuration for the client.
type Config struct {
	APIKey string
	Model  string
}

// NewClient creates a new WebRTC-based Realtime client.
func NewClient(cfg Config) (*Client, error) {
	model := cfg.Model
	if model == "" {
		model = DefaultModel
	}

	return &Client{
		apiKey:     cfg.APIKey,
		model:      model,
		sessionMgr: NewSessionManager(cfg.APIKey),
		msgChan:    make(chan ClientEvent, 100),
		errChan:    make(chan error, 1),
		done:       make(chan struct{}),
	}, nil
}

// OnDataChannelOpen sets a callback to be called when the data channel opens.
// This should be called before Connect().
func (c *Client) OnDataChannelOpen(callback func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onDataChannelOpen = callback
}

// Connect establishes WebRTC connection to OpenAI Realtime API.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Step 1: Create ephemeral transcription session
	slog.Info("creating OpenAI realtime transcription session")
	secretResp, err := c.sessionMgr.CreateSession(ctx, SessionConfig{
		Language: "en", // Default to English, can be made configurable
	})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	slog.Info("session created", "expires", time.Unix(secretResp.ExpiresAt, 0), "keyLen", len(secretResp.Value))

	// Step 2: Create peer connection with media engine
	mediaEngine := &webrtc.MediaEngine{}

	// Register default codecs
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return fmt.Errorf("register codecs: %w", err)
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	pc, err := api.NewPeerConnection(config)
	if err != nil {
		return fmt.Errorf("create peer connection: %w", err)
	}
	c.peerConnection = pc

	// Step 3: Create and add audio track BEFORE creating offer
	// WebRTC Opus requires 48kHz stereo (standard)
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
		return fmt.Errorf("create audio track: %w", err)
	}
	c.audioTrack = audioTrack

	// Add the track to peer connection before creating offer
	_, err = pc.AddTrack(audioTrack)
	if err != nil {
		return fmt.Errorf("add audio track: %w", err)
	}

	// Initialize Opus encoder for 48kHz stereo (matching WebRTC track)
	opusEnc, err := opuscodec.NewEncoder(48000, 2, opuscodec.AppVoIP)
	if err != nil {
		return fmt.Errorf("create opus encoder: %w", err)
	}
	c.opusEncoder = opusEnc

	slog.Info("audio track and opus encoder initialized")

	// Step 4: Create data channel for control messages
	dc, err := pc.CreateDataChannel("oai-events", nil)
	if err != nil {
		return fmt.Errorf("create data channel: %w", err)
	}
	c.dataChannel = dc

	// Set up data channel event handlers
	dc.OnOpen(func() {
		slog.Info("data channel opened")

		// Call the callback if set
		c.mu.Lock()
		callback := c.onDataChannelOpen
		c.mu.Unlock()

		if callback != nil {
			go callback()
		}
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		// Log raw message for debugging
		slog.Info("data channel received message", "length", len(msg.Data), "isString", msg.IsString)

		var event ClientEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			slog.Error("failed to unmarshal event", "error", err, "raw", string(msg.Data[:min(200, len(msg.Data))]))
			return
		}

		slog.Info("parsed event", "type", event.Type)

		select {
		case c.msgChan <- event:
		case <-time.After(100 * time.Millisecond):
			slog.Warn("msg channel full, dropping event", "type", event.Type)
		}
	})

	dc.OnClose(func() {
		slog.Info("data channel closed")
	})

	// Step 5: Handle incoming audio tracks from OpenAI
	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		slog.Info("received remote track",
			"kind", track.Kind(),
			"codec", track.Codec().MimeType,
			"id", track.ID())

		// Read and discard audio packets (we only care about text transcriptions)
		go func() {
			for {
				_, _, err := track.ReadRTP()
				if err != nil {
					return
				}
			}
		}()
	})

	// Step 6: Handle ICE connection state changes
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		slog.Info("ICE connection state changed", "state", state.String())
		if state == webrtc.ICEConnectionStateFailed || state == webrtc.ICEConnectionStateClosed {
			select {
			case c.errChan <- fmt.Errorf("ICE connection %s", state.String()):
			default:
			}
		}
	})

	// Step 7: Create SDP offer
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("create offer: %w", err)
	}

	if err := pc.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("set local description: %w", err)
	}

	// Wait for ICE gathering to complete so that candidates are included in the SDP
	<-webrtc.GatheringCompletePromise(pc)

	// Step 8: Exchange SDP with OpenAI
	// Send our SDP offer to OpenAI and receive their SDP answer
	// Note: We MUST use pc.LocalDescription().SDP here to include the ICE candidates
	localSDP := pc.LocalDescription().SDP
	slog.Info("exchanging SDP with OpenAI")
	slog.Debug("local SDP offer", "sdp", localSDP)
	answerSDP, err := c.sessionMgr.ExchangeSDP(ctx, localSDP, secretResp.Value)
	if err != nil {
		return fmt.Errorf("exchange SDP: %w", err)
	}

	// Step 9: Set OpenAI's SDP answer as remote description
	answer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answerSDP,
	}

	slog.Debug("remote SDP answer", "sdp", answerSDP)
	if err := pc.SetRemoteDescription(answer); err != nil {
		return fmt.Errorf("set remote description: %w", err)
	}

	slog.Info("WebRTC connection established")
	return nil
}

// Send sends a control event via data channel.
func (c *Client) Send(ctx context.Context, event interface{}) error {
	c.mu.Lock()
	dc := c.dataChannel
	c.mu.Unlock()

	if dc == nil {
		slog.Error("data channel is nil")
		return fmt.Errorf("data channel not initialized")
	}

	state := dc.ReadyState()
	slog.Debug("data channel state", "state", state.String())

	if state != webrtc.DataChannelStateOpen {
		return fmt.Errorf("data channel not ready: %s", state.String())
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	slog.Info("sending event via data channel", "size", len(data))

	err = dc.Send(data)
	if err != nil {
		slog.Error("data channel send failed", "error", err)
	}

	return err
}

// SendAudio sends audio samples via the audio track.
// samples should be float32 audio at 48kHz sample rate (mono).
// Converts mono→stereo and encodes to Opus. No sample rate conversion.
func (c *Client) SendAudio(samples []float32) error {
	c.mu.Lock()
	track := c.audioTrack
	encoder := c.opusEncoder
	c.mu.Unlock()

	if track == nil {
		return fmt.Errorf("audio track not ready")
	}
	if encoder == nil {
		return fmt.Errorf("opus encoder not ready")
	}

	// Convert mono to stereo (simple duplication: L=R=sample)
	stereo := make([]float32, len(samples)*2)
	for i, s := range samples {
		stereo[i*2] = s   // Left
		stereo[i*2+1] = s // Right
	}

	// Encode stereo with Opus (48kHz stereo matches WebRTC track)
	// Max Opus packet size is ~4000 bytes, use 1275 per spec
	opusData := make([]byte, 1275)
	n, err := encoder.EncodeFloat32(stereo, opusData)
	if err != nil {
		return fmt.Errorf("opus encode: %w", err)
	}
	opusData = opusData[:n]

	// Calculate duration based on sample count at 48kHz
	sampleDuration := time.Duration(len(samples)) * time.Second / 48000

	sample := media.Sample{
		Data:               opusData,
		Duration:           sampleDuration,
		PacketTimestamp:    0, // Will be set by track
		PrevDroppedPackets: 0,
	}

	err = track.WriteSample(sample)
	if err != nil {
		slog.Warn("WriteSample failed", "error", err)
	}

	return err
}

// Messages returns the channel for receiving events.
func (c *Client) Messages() <-chan ClientEvent {
	return c.msgChan
}

// Errors returns the channel for receiving errors.
func (c *Client) Errors() <-chan error {
	return c.errChan
}

// Close closes the WebRTC connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	close(c.done)

	if c.dataChannel != nil {
		_ = c.dataChannel.Close()
	}

	if c.peerConnection != nil {
		return c.peerConnection.Close()
	}

	return nil
}

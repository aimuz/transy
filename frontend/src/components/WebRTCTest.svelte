<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { Events } from '@wailsio/runtime'
  import { startAudioCapture, stopAudioCapture } from '../services/wails'

  let webrtcSupported = $state(false)
  let microphoneSupported = $state(false)
  let displayMediaSupported = $state(false)

  let testResults = $state<string[]>([])
  let audioStream: MediaStream | null = null
  let isCapturing = $state(false)
  let audioLevel = $state(0)
  let analyser: AnalyserNode | null = null
  let animationId: number | null = null

  // Go audio bridge state
  let isGoCapturing = $state(false)
  let goAudioLevel = $state(0)
  let goSampleCount = $state(0)
  let eventUnsubscribe: (() => void) | null = null

  // Real-time metrics
  let avgLatency = $state(0) // Average latency in ms
  let jitter = $state(0) // Jitter (variance in arrival times) in ms
  let chunkFrequency = $state(0) // Chunks per second
  let samplesPerChunk = $state(0) // Samples in each chunk
  let packetLoss = $state(0) // Lost packets count

  // Metrics calculation state
  let latencyHistory: number[] = []
  let arrivalTimes: number[] = []
  let lastSeq = 0
  let frequencyStartTime = 0
  let frequencyChunkCount = 0

  interface AudioEvent {
    samples: number[]
    timestamp: number
    seq: number
  }

  onMount(() => {
    // Listen for audio samples from Go backend
    eventUnsubscribe = Events.On('audio-samples', (event: { data: unknown }) => {
      const now = Date.now()
      const data = event.data as AudioEvent
      goSampleCount++

      // Parse structured data
      const samples = data.samples
      const goTimestamp = data.timestamp
      const seq = data.seq

      // Calculate audio level from samples
      if (samples && samples.length > 0) {
        let maxAmp = 0
        for (const sample of samples) {
          const abs = Math.abs(sample)
          if (abs > maxAmp) maxAmp = abs
        }
        goAudioLevel = maxAmp * 100
        samplesPerChunk = samples.length
      }

      // Calculate latency
      const latency = now - goTimestamp
      latencyHistory.push(latency)
      if (latencyHistory.length > 100) latencyHistory.shift()
      avgLatency = latencyHistory.reduce((a, b) => a + b, 0) / latencyHistory.length

      // Calculate jitter (standard deviation of inter-arrival times)
      arrivalTimes.push(now)
      if (arrivalTimes.length > 100) arrivalTimes.shift()
      if (arrivalTimes.length > 1) {
        const intervals: number[] = []
        for (let i = 1; i < arrivalTimes.length; i++) {
          intervals.push(arrivalTimes[i] - arrivalTimes[i - 1])
        }
        const avgInterval = intervals.reduce((a, b) => a + b, 0) / intervals.length
        const variance =
          intervals.reduce((sum, val) => sum + Math.pow(val - avgInterval, 2), 0) / intervals.length
        jitter = Math.sqrt(variance)
      }

      // Calculate chunk frequency (chunks per second)
      if (frequencyStartTime === 0) {
        frequencyStartTime = now
      }
      frequencyChunkCount++
      const elapsed = (now - frequencyStartTime) / 1000
      if (elapsed >= 1) {
        chunkFrequency = frequencyChunkCount / elapsed
        frequencyStartTime = now
        frequencyChunkCount = 0
      }

      // Detect packet loss
      if (lastSeq > 0 && seq > lastSeq + 1) {
        packetLoss += seq - lastSeq - 1
      }
      lastSeq = seq

      if (goSampleCount % 100 === 0) {
        testResults.push(
          `🎵 Go Audio: seq=${seq}, latency=${avgLatency.toFixed(1)}ms, freq=${chunkFrequency.toFixed(0)}/s`
        )
        testResults = testResults
      }
    })
  })

  onDestroy(() => {
    if (eventUnsubscribe) {
      eventUnsubscribe()
    }
  })

  // Start Go audio capture
  async function startGoAudioCapture() {
    try {
      await startAudioCapture()
      isGoCapturing = true
      goSampleCount = 0
      testResults.push('✅ Go Audio Capture started')
      testResults = testResults
    } catch (e: any) {
      testResults.push(`❌ Go Audio Error: ${e}`)
      testResults = testResults
    }
  }

  // Stop Go audio capture
  async function stopGoAudioCapture() {
    try {
      await stopAudioCapture()
      isGoCapturing = false
      goAudioLevel = 0
      testResults.push(`⏹️ Go Audio Capture stopped (${goSampleCount} chunks received)`)
      testResults = testResults
    } catch (e: any) {
      testResults.push(`❌ Go Audio Stop Error: ${e}`)
      testResults = testResults
    }
  }

  // Test WebRTC support
  function testWebRTC() {
    try {
      webrtcSupported = !!window.RTCPeerConnection
      testResults.push(`✅ RTCPeerConnection: ${webrtcSupported ? 'Supported' : 'Not supported'}`)

      // Check DataChannel support
      if (webrtcSupported) {
        const pc = new RTCPeerConnection()
        const dc = pc.createDataChannel('test')
        testResults.push(`✅ DataChannel: Supported`)
        dc.close()
        pc.close()
      }
    } catch (e) {
      testResults.push(`❌ WebRTC Error: ${e}`)
    }
    testResults = testResults
    console.log(testResults)
  }

  // Test microphone access
  async function testMicrophone() {
    try {
      microphoneSupported = !!navigator.mediaDevices?.getUserMedia
      testResults.push(
        `✅ getUserMedia API: ${microphoneSupported ? 'Available' : 'Not available'}`
      )

      if (microphoneSupported) {
        const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
        testResults.push(`✅ Microphone: Access granted`)
        stream.getTracks().forEach((track) => track.stop())
      }
    } catch (e: any) {
      testResults.push(`❌ Microphone Error: ${e.name} - ${e.message}`)
    }
    testResults = testResults
  }

  // Test screen/system audio capture (getDisplayMedia)
  async function testDisplayMedia() {
    try {
      displayMediaSupported = !!navigator.mediaDevices?.getDisplayMedia
      testResults.push(
        `✅ getDisplayMedia API: ${displayMediaSupported ? 'Available' : 'Not available'}`
      )

      if (displayMediaSupported) {
        // Try to capture with system audio
        const stream = await navigator.mediaDevices.getDisplayMedia({
          video: true, // Required for getDisplayMedia
          audio: {
            // 采样率 (Hz)
            sampleRate: 48000,

            // 声道数
            channelCount: 2,

            // 采样大小 (bits)
            sampleSize: 16,
          }, // System audio (if supported)
        })

        const audioTracks = stream.getAudioTracks()
        const videoTracks = stream.getVideoTracks()

        testResults.push(`✅ Screen Capture: Success`)
        testResults.push(`   - Video tracks: ${videoTracks.length}`)
        testResults.push(
          `   - Audio tracks: ${audioTracks.length} ${audioTracks.length > 0 ? '(System audio!)' : '(No audio)'}`
        )

        stream.getTracks().forEach((track) => track.stop())
      }
    } catch (e: any) {
      testResults.push(`❌ DisplayMedia Error: ${e.name} - ${e.message}`)
    }
    testResults = testResults
    console.log(testResults)
  }

  // Start capturing system audio (browser)
  async function startSystemAudioCapture() {
    try {
      const stream = await navigator.mediaDevices.getDisplayMedia({
        video: true,
        audio: true,
      })

      // Check if we got audio
      const audioTracks = stream.getAudioTracks()
      if (audioTracks.length === 0) {
        testResults.push(
          `⚠️ No audio track in stream. Make sure to enable "Share audio" in the dialog.`
        )
        console.log(testResults)
        stream.getTracks().forEach((track) => track.stop())
        testResults = testResults
        return
      }

      audioStream = stream
      isCapturing = true
      testResults.push(`🎵 Capturing system audio...`)
      console.log(testResults)
      testResults = testResults

      // Create audio analyzer to show audio level
      const audioContext = new AudioContext()
      const source = audioContext.createMediaStreamSource(stream)
      analyser = audioContext.createAnalyser()
      analyser.fftSize = 256
      source.connect(analyser)

      // Start audio level visualization
      updateAudioLevel()
    } catch (e: any) {
      testResults.push(`❌ Capture Error: ${e.name} - ${e.message}`)
      testResults = testResults
    }
    console.log(testResults)
  }

  function updateAudioLevel() {
    if (!analyser || !isCapturing) return

    const dataArray = new Uint8Array(analyser.frequencyBinCount)
    analyser.getByteFrequencyData(dataArray)

    // Calculate average level
    const sum = dataArray.reduce((a, b) => a + b, 0)
    audioLevel = sum / dataArray.length

    animationId = requestAnimationFrame(updateAudioLevel)
  }

  function stopCapture() {
    if (audioStream) {
      audioStream.getTracks().forEach((track) => track.stop())
      audioStream = null
    }
    if (animationId) {
      cancelAnimationFrame(animationId)
      animationId = null
    }
    isCapturing = false
    audioLevel = 0
    testResults.push(`⏹️ Capture stopped`)
    testResults = testResults
  }

  function runAllTests() {
    testResults = []
    testResults.push('=== WebRTC Compatibility Test ===')
    testResults.push(`User Agent: ${navigator.userAgent.substring(0, 80)}...`)
    testResults = testResults
    testWebRTC()
  }
</script>

<div class="test-container">
  <h2>🔬 WebRTC Compatibility Test</h2>

  <div class="button-group">
    <button onclick={runAllTests}>1. Test WebRTC</button>
    <button onclick={testMicrophone}>2. Test Microphone</button>
    <button onclick={testDisplayMedia}>3. Test Screen Capture</button>
  </div>

  <div class="button-group">
    {#if !isCapturing}
      <button onclick={startSystemAudioCapture} class="primary">
        🎵 Start System Audio Capture
      </button>
    {:else}
      <button onclick={stopCapture} class="danger"> ⏹️ Stop Capture </button>
    {/if}
  </div>

  {#if isCapturing}
    <div class="audio-meter">
      <div class="audio-level" style="width: {Math.min(audioLevel * 2, 100)}%"></div>
    </div>
    <p class="audio-label">Audio Level: {audioLevel.toFixed(0)}</p>
  {/if}

  <h3>🔗 Go Audio Bridge (ScreenCaptureKit)</h3>
  <div class="button-group">
    {#if !isGoCapturing}
      <button onclick={startGoAudioCapture} class="primary"> 🎧 Start Go Audio Capture </button>
    {:else}
      <button onclick={stopGoAudioCapture} class="danger"> ⏹️ Stop Go Capture </button>
    {/if}
  </div>

  {#if isGoCapturing}
    <div class="audio-meter">
      <div class="audio-level" style="width: {Math.min(goAudioLevel, 100)}%"></div>
    </div>

    <div class="metrics-grid">
      <div class="metric">
        <span class="metric-label">⏱️ Latency</span>
        <span class="metric-value">{avgLatency.toFixed(1)} ms</span>
      </div>
      <div class="metric">
        <span class="metric-label">📊 Jitter</span>
        <span class="metric-value">{jitter.toFixed(2)} ms</span>
      </div>
      <div class="metric">
        <span class="metric-label">🔄 Frequency</span>
        <span class="metric-value">{chunkFrequency.toFixed(0)} /s</span>
      </div>
      <div class="metric">
        <span class="metric-label">📦 Samples/Chunk</span>
        <span class="metric-value">{samplesPerChunk}</span>
      </div>
      <div class="metric">
        <span class="metric-label">📶 Audio Level</span>
        <span class="metric-value">{goAudioLevel.toFixed(1)}%</span>
      </div>
      <div class="metric">
        <span class="metric-label">❌ Packet Loss</span>
        <span class="metric-value" class:error={packetLoss > 0}>{packetLoss}</span>
      </div>
    </div>
    <p class="audio-label">Chunks received: {goSampleCount}</p>
  {/if}

  <div class="results">
    {#each testResults as result}
      <div class="result-line">{result}</div>
    {/each}
  </div>
</div>

<style>
  .test-container {
    padding: 20px;
    background: var(--surface-secondary, #1a1a1a);
    border-radius: 12px;
    margin: 20px;
  }

  h2 {
    margin-bottom: 20px;
    color: var(--text-primary, #fff);
  }

  .button-group {
    display: flex;
    gap: 10px;
    margin-bottom: 15px;
    flex-wrap: wrap;
  }

  button {
    padding: 10px 20px;
    border-radius: 8px;
    border: none;
    background: var(--surface-tertiary, #333);
    color: var(--text-primary, #fff);
    cursor: pointer;
    font-size: 14px;
    transition: all 0.2s;
  }

  button:hover {
    background: var(--surface-hover, #444);
  }

  button.primary {
    background: var(--accent-primary, #0066ff);
  }

  button.primary:hover {
    background: var(--accent-hover, #0055dd);
  }

  button.danger {
    background: #dc3545;
  }

  button.danger:hover {
    background: #c82333;
  }

  .results {
    background: var(--surface-primary, #0d0d0d);
    border-radius: 8px;
    padding: 15px;
    font-family: monospace;
    font-size: 13px;
    max-height: 400px;
    overflow-y: auto;
  }

  .result-line {
    padding: 4px 0;
    color: var(--text-secondary, #aaa);
    white-space: pre-wrap;
  }

  .audio-meter {
    height: 20px;
    background: var(--surface-primary, #0d0d0d);
    border-radius: 10px;
    overflow: hidden;
    margin: 10px 0;
  }

  .audio-level {
    height: 100%;
    background: linear-gradient(90deg, #00ff88, #ffff00, #ff4444);
    transition: width 0.05s;
  }

  .audio-label {
    text-align: center;
    color: var(--text-secondary, #aaa);
    font-size: 12px;
  }

  .metrics-grid {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 10px;
    margin: 15px 0;
  }

  .metric {
    background: var(--surface-primary, #0d0d0d);
    border-radius: 8px;
    padding: 12px;
    text-align: center;
  }

  .metric-label {
    display: block;
    font-size: 11px;
    color: var(--text-secondary, #888);
    margin-bottom: 4px;
  }

  .metric-value {
    display: block;
    font-size: 18px;
    font-weight: 600;
    color: var(--text-primary, #fff);
    font-family: 'SF Mono', monospace;
  }

  .metric-value.error {
    color: #ff4444;
  }

  h3 {
    margin: 20px 0 10px;
    color: var(--text-primary, #fff);
    font-size: 14px;
  }
</style>

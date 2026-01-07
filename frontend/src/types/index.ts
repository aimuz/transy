// Re-export types from Wails generated models
// export { llm, main } from '../../wailsjs/go/models'

export type Provider = {
  name: string
  type: 'openai' | 'openai-compatible' | 'gemini' | 'claude'
  base_url?: string
  api_key: string
  model: string
  system_prompt?: string
  max_tokens?: number
  temperature?: number
  active: boolean
  disable_thinking?: boolean // For Gemini: set thinkingBudget to 0
}

export type TranslateRequest = {
  text: string
  sourceLang: string
  targetLang: string
}

export type DetectLanguageResponse = {
  code: string
  name: string
  defaultTarget: string
}

export type Usage = {
  promptTokens: number
  completionTokens: number
  totalTokens: number
  cacheHit: boolean
}

export type TranslateResult = {
  text: string
  usage: Usage
}

export type Language = {
  code: string
  name: string
}

export const LANGUAGES: Language[] = [
  { code: 'auto', name: '自动' },
  { code: 'zh', name: '中文' },
  { code: 'en', name: '英语' },
  { code: 'ja', name: '日语' },
  { code: 'ko', name: '韩语' },
  { code: 'fr', name: '法语' },
  { code: 'de', name: '德语' },
  { code: 'es', name: '西班牙语' },
  { code: 'ru', name: '俄语' },
  { code: 'it', name: '意大利语' },
  { code: 'pt', name: '葡萄牙语' },
  { code: 'ar', name: '阿拉伯语' },
]

export const LANGUAGE_NAME_MAP: Record<string, string> = Object.fromEntries(
  LANGUAGES.map((l) => [l.name, l.code])
)

export const LANGUAGE_CODE_MAP: Record<string, string> = Object.fromEntries(
  LANGUAGES.map((l) => [l.code, l.name])
)

// ─────────────────────────────────────────────────────────────────────────────
// Live Translation Types
// ─────────────────────────────────────────────────────────────────────────────

export type LiveTranscript = {
  id: string
  sourceText: string
  targetText: string
  sourceLang: string
  targetLang: string
  startTime: number
  endTime: number
  timestamp: number
  isFinal: boolean
  confidence: number
}

export type VADState = 'listening' | 'speaking' | 'processing'

export type LiveStatus = {
  active: boolean
  sourceLang: string
  targetLang: string
  duration: number
  sttProvider: string
  transcriptCount: number
  vadState: VADState
}

export type STTProviderInfo = {
  name: string
  displayName: string
  isLocal: boolean
  requiresSetup: boolean
  setupProgress: number
  isReady: boolean
}

// ─────────────────────────────────────────────────────────────────────────────
// New Configuration Architecture
// ─────────────────────────────────────────────────────────────────────────────

export type APICredential = {
  id: string
  name: string
  type: 'openai' | 'openai-compatible' | 'gemini' | 'claude'
  base_url?: string
  api_key: string
}

export type TranslationProfile = {
  id: string
  name: string
  credential_id: string
  model: string
  system_prompt?: string
  max_tokens?: number
  temperature?: number
  active: boolean
  disable_thinking?: boolean
}

// ─────────────────────────────────────────────────────────────────────────────
// Transcription Models
// ─────────────────────────────────────────────────────────────────────────────

export type TranscriptionModel = {
  id: string
  name: string
  description: string
}

export const TRANSCRIPTION_MODELS: TranscriptionModel[] = [
  { id: 'gpt-4o-transcribe', name: 'GPT-4o Transcribe', description: '推荐 - 高精度转录' },
  { id: 'gpt-4o-transcribe-diarize', name: 'GPT-4o Transcribe + Diarize', description: '支持说话人识别' },
  { id: 'gpt-4o-mini-transcribe', name: 'GPT-4o Mini Transcribe', description: '更快速、更低成本' },
  { id: 'whisper-1', name: 'Whisper-1', description: '经典语音识别模型' },
]

export type SpeechConfig = {
  enabled: boolean
  credential_id?: string
  model?: string
  mode?: 'transcription' | 'realtime'
}

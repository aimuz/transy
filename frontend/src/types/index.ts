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
  text: string
  translated: string
  timestamp: number
  isFinal: boolean
  confidence: number
}

export type LiveStatus = {
  active: boolean
  sourceLang: string
  targetLang: string
  duration: number
  sttProvider: string
  transcriptCount: number
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

export type SpeechConfig = {
  enabled: boolean
  credential_id?: string
  model?: string
  mode?: 'transcription' | 'realtime'
}

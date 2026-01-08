// Wails v3 bindings
// @ts-ignore - Auto-generated Wails bindings without type declarations
import * as App from '../../bindings/go.aimuz.me/transy/internal/app/service.js'
import type { TranslateRequest, DetectLanguageResponse, TranslateResult } from '../types'

// Translation
export async function translateWithLLM(request: TranslateRequest): Promise<TranslateResult> {
  return await App.TranslateWithLLM(request)
}

// Streaming translation - results come via 'translate-chunk' events
export async function translateWithLLMStream(request: TranslateRequest): Promise<void> {
  await App.TranslateWithLLMStream(request)
}

export async function detectLanguage(text: string): Promise<DetectLanguageResponse> {
  return await App.DetectLanguage(text)
}

// Language settings
export async function getDefaultLanguages(): Promise<Record<string, string>> {
  const langs = await App.GetDefaultLanguages()
  return langs || {}
}

export async function setDefaultLanguage(sourceLang: string, targetLang: string): Promise<void> {
  await App.SetDefaultLanguage(sourceLang, targetLang)
}

// Window
export async function toggleWindowVisibility(): Promise<void> {
  await App.ToggleWindowVisibility()
}

// Accessibility
export async function getAccessibilityPermission(): Promise<boolean> {
  return await App.GetAccessibilityPermission()
}

// OCR
export async function takeScreenshotAndOCR(): Promise<string> {
  return await App.TakeScreenshotAndOCR()
}

// Version
export async function getVersion(): Promise<string> {
  return await App.GetVersion()
}

// ─────────────────────────────────────────────────────────────────────────────
// Live Translation
// ─────────────────────────────────────────────────────────────────────────────

import type { LiveStatus } from '../types'

export async function startLiveTranslation(sourceLang: string, targetLang: string): Promise<void> {
  await App.StartLiveTranslation(sourceLang, targetLang)
}

export async function stopLiveTranslation(): Promise<void> {
  await App.StopLiveTranslation()
}

export async function getLiveStatus(): Promise<LiveStatus> {
  return (await App.GetLiveStatus()) as LiveStatus
}

// ─────────────────────────────────────────────────────────────────────────────
// New Configuration Architecture
// ─────────────────────────────────────────────────────────────────────────────

import type { APICredential, TranslationProfile, SpeechConfig } from '../types'

// API Credentials
export async function getCredentials(): Promise<APICredential[]> {
  const credentials = await App.GetCredentials()
  return (credentials || []) as APICredential[]
}

export async function addCredential(credential: APICredential): Promise<void> {
  await App.AddCredential(credential)
}

export async function updateCredential(id: string, credential: APICredential): Promise<void> {
  await App.UpdateCredential(id, credential)
}

export async function removeCredential(id: string): Promise<void> {
  await App.RemoveCredential(id)
}

// Translation Profiles
export async function getTranslationProfiles(): Promise<TranslationProfile[]> {
  const profiles = await App.GetTranslationProfiles()
  return (profiles || []) as TranslationProfile[]
}

export async function getActiveTranslationProfile(): Promise<TranslationProfile | null> {
  return (await App.GetActiveTranslationProfile()) as TranslationProfile | null
}

export async function addTranslationProfile(profile: TranslationProfile): Promise<void> {
  await App.AddTranslationProfile(profile)
}

export async function updateTranslationProfile(id: string, profile: TranslationProfile): Promise<void> {
  await App.UpdateTranslationProfile(id, profile)
}

export async function removeTranslationProfile(id: string): Promise<void> {
  await App.RemoveTranslationProfile(id)
}

export async function setTranslationProfileActive(id: string): Promise<void> {
  await App.SetTranslationProfileActive(id)
}

// Speech Config
export async function getSpeechConfig(): Promise<SpeechConfig | null> {
  return (await App.GetSpeechConfig()) as SpeechConfig | null
}

export async function setSpeechConfig(config: SpeechConfig): Promise<void> {
  await App.SetSpeechConfig(config)
}

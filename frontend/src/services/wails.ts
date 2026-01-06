// Wails v3 bindings
import * as App from '../../bindings/go.aimuz.me/transy/app.js'
import type { Provider, TranslateRequest, DetectLanguageResponse, TranslateResult } from '../types'

// Provider management
export async function getProviders(): Promise<Provider[]> {
  const providers = await App.GetProviders()
  return (providers || []) as Provider[]
}

export async function addProvider(provider: Provider): Promise<void> {
  await App.AddProvider(provider)
}

export async function updateProvider(oldName: string, provider: Provider): Promise<void> {
  await App.UpdateProvider(oldName, provider)
}

export async function removeProvider(name: string): Promise<void> {
  await App.RemoveProvider(name)
}

export async function setProviderActive(name: string): Promise<void> {
  await App.SetProviderActive(name)
}

export async function getActiveProvider(): Promise<Provider | null> {
  return (await App.GetActiveProvider()) as Provider | null
}

// Translation
export async function translateWithLLM(request: TranslateRequest): Promise<TranslateResult> {
  return await App.TranslateWithLLM(request)
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

import type { LiveStatus, STTProviderInfo } from '../types'

export async function startLiveTranslation(sourceLang: string, targetLang: string): Promise<void> {
  await App.StartLiveTranslation(sourceLang, targetLang)
}

export async function stopLiveTranslation(): Promise<void> {
  await App.StopLiveTranslation()
}

export async function getLiveStatus(): Promise<LiveStatus> {
  return (await App.GetLiveStatus()) as LiveStatus
}

export async function getSTTProviders(): Promise<STTProviderInfo[]> {
  const providers = await App.GetSTTProviders()
  return (providers || []) as STTProviderInfo[]
}

export async function setSTTProvider(name: string): Promise<void> {
  await App.SetSTTProvider(name)
}

export async function setupSTTProvider(name: string): Promise<void> {
  await App.SetupSTTProvider(name)
}

export async function getSTTSetupProgress(name: string): Promise<number> {
  return await App.GetSTTSetupProgress(name)
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

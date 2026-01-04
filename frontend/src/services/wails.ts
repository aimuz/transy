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

// speech_darwin.m - Objective-C implementation for macOS Speech Framework STT

#import <Speech/Speech.h>
#import <Foundation/Foundation.h>
#import <AVFoundation/AVFoundation.h>
#include <stdlib.h>

// Helper class to handle authorization request
API_AVAILABLE(macos(10.15))
@interface SpeechAuthHelper : NSObject
+ (void)requestAuth;
@end

API_AVAILABLE(macos(10.15))
@implementation SpeechAuthHelper
+ (void)requestAuth {
    [SFSpeechRecognizer requestAuthorization:^(SFSpeechRecognizerAuthorizationStatus status) {
        NSLog(@"Speech authorization request completed. New Status: %ld", (long)status);
    }];
}
@end

// Check if speech recognition is available for a given locale
int isSpeechRecognitionAvailable(const char* locale) {
    if (@available(macOS 10.15, *)) {
        NSLocale *nsLocale = [NSLocale localeWithLocaleIdentifier:[NSString stringWithUTF8String:locale]];
        SFSpeechRecognizer *recognizer = [[SFSpeechRecognizer alloc] initWithLocale:nsLocale];
        return recognizer.isAvailable ? 1 : 0;
    }
    return 0;
}

// Request speech recognition authorization
// Returns: 1 = authorized, 0 = denied/restricted/not determined
// Note: This schedules the authorization request to run on the main run loop after a delay.
int requestSpeechRecognitionAuthorization(void) {
    if (@available(macOS 10.15, *)) {
        // Check current authorization status first (non-blocking)
        SFSpeechRecognizerAuthorizationStatus currentStatus = [SFSpeechRecognizer authorizationStatus];
        if (currentStatus == SFSpeechRecognizerAuthorizationStatusNotDetermined) {
            // Schedule the authorization request to run on the main run loop after a delay
            // Using performSelector:afterDelay: ensures it runs during the next run loop iteration
            [SpeechAuthHelper performSelectorOnMainThread:@selector(requestAuth) 
                                               withObject:nil 
                                            waitUntilDone:NO];
            return 0; // Triggered, but not ready
        }
        
        return (currentStatus == SFSpeechRecognizerAuthorizationStatusAuthorized) ? 1 : 0;
    }
    return 0;
}

int checkSpeechRecognitionAuthorizationStatus(void) {
    if (@available(macOS 10.15, *)) {
        return (int)[SFSpeechRecognizer authorizationStatus];
    }
    return 0; // NotDetermined
}

// Recognize speech from audio samples
// samples: PCM float32 audio samples
// sampleCount: number of samples
// sampleRate: sample rate (e.g., 16000)
// locale: locale identifier (e.g., "en-US", "zh-CN")
// Returns: recognized text (caller must free) or NULL
// Note: This function must NOT be called from the main thread if the main thread is
// running AppKit, as it would deadlock. The caller (Go) should ensure this runs
// on a background goroutine.
char* recognizeSpeech(float* samples, int sampleCount, int sampleRate, const char* locale) {
    if (@available(macOS 10.15, *)) {
        // Copy samples to heap since we may outlive the stack frame in async dispatch
        float *samplesCopy = malloc(sampleCount * sizeof(float));
        if (!samplesCopy) {
            NSLog(@"Failed to allocate memory for samples");
            return NULL;
        }
        memcpy(samplesCopy, samples, sampleCount * sizeof(float));
        
        NSString *localeStr = [NSString stringWithUTF8String:locale];
        
        __block char *result = NULL;
        dispatch_semaphore_t doneSemaphore = dispatch_semaphore_create(0);
        
        // Dispatch the recognition work to a background queue to avoid main thread issues
        dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
            @autoreleasepool {
                // Create locale and recognizer
                NSLocale *nsLocale = [NSLocale localeWithLocaleIdentifier:localeStr];
                SFSpeechRecognizer *recognizer = [[SFSpeechRecognizer alloc] initWithLocale:nsLocale];
                
                if (!recognizer || !recognizer.isAvailable) {
                    NSLog(@"Speech recognizer not available for locale: %@", localeStr);
                    free(samplesCopy);
                    dispatch_semaphore_signal(doneSemaphore);
                    return;
                }
                
                // Convert float samples to audio buffer
                // Create audio format for float32 mono
                AVAudioFormat *format = [[AVAudioFormat alloc] initWithCommonFormat:AVAudioPCMFormatFloat32
                                                                         sampleRate:sampleRate
                                                                           channels:1
                                                                        interleaved:NO];
                
                // Create audio buffer
                AVAudioPCMBuffer *buffer = [[AVAudioPCMBuffer alloc] initWithPCMFormat:format
                                                                         frameCapacity:sampleCount];
                buffer.frameLength = sampleCount;
                
                // Copy samples to buffer
                float *channelData = buffer.floatChannelData[0];
                memcpy(channelData, samplesCopy, sampleCount * sizeof(float));
                free(samplesCopy);
                
                // Write to temp file (SFSpeechRecognizer requires file or audio buffer from file)
                NSString *tempPath = [NSTemporaryDirectory() stringByAppendingPathComponent:
                                      [NSString stringWithFormat:@"speech_%lld.wav", (long long)[NSDate date].timeIntervalSince1970 * 1000]];
                NSURL *tempURL = [NSURL fileURLWithPath:tempPath];
                
                // Create audio file
                NSError *error = nil;
                AVAudioFile *audioFile = [[AVAudioFile alloc] initForWriting:tempURL
                                                                    settings:format.settings
                                                                       error:&error];
                if (error) {
                    NSLog(@"Failed to create audio file: %@", error);
                    dispatch_semaphore_signal(doneSemaphore);
                    return;
                }
                
                // Write buffer to file
                [audioFile writeFromBuffer:buffer error:&error];
                if (error) {
                    NSLog(@"Failed to write audio buffer: %@", error);
                    [[NSFileManager defaultManager] removeItemAtURL:tempURL error:nil];
                    dispatch_semaphore_signal(doneSemaphore);
                    return;
                }
                audioFile = nil; // Close file
                
                // Create recognition request
                SFSpeechURLRecognitionRequest *request = [[SFSpeechURLRecognitionRequest alloc] initWithURL:tempURL];
                request.shouldReportPartialResults = NO;
                
                // Enable on-device recognition if available (lower latency)
                if (@available(macOS 13.0, *)) {
                    request.requiresOnDeviceRecognition = recognizer.supportsOnDeviceRecognition;
                }
                
                dispatch_semaphore_t recognitionSemaphore = dispatch_semaphore_create(0);
                __block NSString *resultText = nil;
                
                // Perform recognition
                [recognizer recognitionTaskWithRequest:request resultHandler:^(SFSpeechRecognitionResult * _Nullable res, NSError * _Nullable err) {
                    if (err) {
                        NSLog(@"Speech recognition error: %@", err);
                    }
                    
                    if (res.isFinal) {
                        resultText = [res.bestTranscription.formattedString copy];
                        dispatch_semaphore_signal(recognitionSemaphore);
                    }
                }];
                
                // Wait for recognition result with timeout (on this background queue, safe)
                dispatch_time_t timeout = dispatch_time(DISPATCH_TIME_NOW, 30 * NSEC_PER_SEC);
                dispatch_semaphore_wait(recognitionSemaphore, timeout);
                
                // Clean up temp file
                [[NSFileManager defaultManager] removeItemAtURL:tempURL error:nil];
                
                if (resultText && resultText.length > 0) {
                    result = strdup([resultText UTF8String]);
                }
                
                dispatch_semaphore_signal(doneSemaphore);
            }
        });
        
        // Wait for the background work to complete
        dispatch_time_t outerTimeout = dispatch_time(DISPATCH_TIME_NOW, 35 * NSEC_PER_SEC);
        dispatch_semaphore_wait(doneSemaphore, outerTimeout);
        
        return result;
    }
    return NULL;
}

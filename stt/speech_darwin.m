// speech_darwin.m - Objective-C implementation for macOS Speech Framework STT

#import <Speech/Speech.h>
#import <Foundation/Foundation.h>
#import <AVFoundation/AVFoundation.h>
#include <stdlib.h>

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
// Note: This function dispatches the wait to a background queue to avoid main thread deadlock.
int requestSpeechRecognitionAuthorization(void) {
    if (@available(macOS 10.15, *)) {
        // Check current authorization status first (non-blocking)
        SFSpeechRecognizerAuthorizationStatus currentStatus = [SFSpeechRecognizer authorizationStatus];
        if (currentStatus == SFSpeechRecognizerAuthorizationStatusAuthorized) {
            return 1;
        }
        if (currentStatus == SFSpeechRecognizerAuthorizationStatusDenied ||
            currentStatus == SFSpeechRecognizerAuthorizationStatusRestricted) {
            return 0;
        }
        
        // Status is NotDetermined - need to request authorization
        // Use dispatch to background queue to avoid main thread deadlock
        __block int result = 0;
        dispatch_semaphore_t semaphore = dispatch_semaphore_create(0);
        
        dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
            [SFSpeechRecognizer requestAuthorization:^(SFSpeechRecognizerAuthorizationStatus status) {
                result = (status == SFSpeechRecognizerAuthorizationStatusAuthorized) ? 1 : 0;
                dispatch_semaphore_signal(semaphore);
            }];
        });
        
        dispatch_semaphore_wait(semaphore, dispatch_time(DISPATCH_TIME_NOW, 10 * NSEC_PER_SEC));
        return result;
    }
    return 0;
}

// Recognize speech from audio samples
// samples: PCM float32 audio samples
// sampleCount: number of samples
// sampleRate: sample rate (e.g., 16000)
// locale: locale identifier (e.g., "en-US", "zh-CN")
// Returns: recognized text (caller must free) or NULL
char* recognizeSpeech(float* samples, int sampleCount, int sampleRate, const char* locale) {
    if (@available(macOS 10.15, *)) {
        @autoreleasepool {
            // Create locale and recognizer
            NSLocale *nsLocale = [NSLocale localeWithLocaleIdentifier:[NSString stringWithUTF8String:locale]];
            SFSpeechRecognizer *recognizer = [[SFSpeechRecognizer alloc] initWithLocale:nsLocale];
            
            if (!recognizer || !recognizer.isAvailable) {
                NSLog(@"Speech recognizer not available for locale: %s", locale);
                return NULL;
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
            memcpy(channelData, samples, sampleCount * sizeof(float));
            
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
                return NULL;
            }
            
            // Write buffer to file
            [audioFile writeFromBuffer:buffer error:&error];
            if (error) {
                NSLog(@"Failed to write audio buffer: %@", error);
                [[NSFileManager defaultManager] removeItemAtURL:tempURL error:nil];
                return NULL;
            }
            audioFile = nil; // Close file
            
            // Create recognition request
            SFSpeechURLRecognitionRequest *request = [[SFSpeechURLRecognitionRequest alloc] initWithURL:tempURL];
            request.shouldReportPartialResults = NO;
            
            // Enable on-device recognition if available (lower latency)
            if (@available(macOS 13.0, *)) {
                request.requiresOnDeviceRecognition = recognizer.supportsOnDeviceRecognition;
            }
            
            __block NSString *resultText = nil;
            __block BOOL completed = NO;
            dispatch_semaphore_t semaphore = dispatch_semaphore_create(0);
            
            // Perform recognition
            [recognizer recognitionTaskWithRequest:request resultHandler:^(SFSpeechRecognitionResult * _Nullable result, NSError * _Nullable error) {
                if (error) {
                    NSLog(@"Speech recognition error: %@", error);
                }
                
                if (result.isFinal) {
                    resultText = result.bestTranscription.formattedString;
                    completed = YES;
                    dispatch_semaphore_signal(semaphore);
                }
            }];
            
            // Wait for result with timeout
            dispatch_time_t timeout = dispatch_time(DISPATCH_TIME_NOW, 30 * NSEC_PER_SEC);
            dispatch_semaphore_wait(semaphore, timeout);
            
            // Clean up temp file
            [[NSFileManager defaultManager] removeItemAtURL:tempURL error:nil];
            
            if (resultText && resultText.length > 0) {
                return strdup([resultText UTF8String]);
            }
            
            return NULL;
        }
    }
    return NULL;
}

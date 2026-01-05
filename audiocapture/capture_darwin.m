// capture_darwin.m - Objective-C implementation for ScreenCaptureKit audio capture

#import <ScreenCaptureKit/ScreenCaptureKit.h>
#import <CoreMedia/CoreMedia.h>
#import <CoreAudio/CoreAudio.h>
#import <Foundation/Foundation.h>
#import <AVFoundation/AVFoundation.h>
#include <stdlib.h>

// Forward declaration of Go callback (implemented in capture_darwin.go)
extern void goAudioCallback(void* context, float* samples, int count);

// Audio capture delegate
API_AVAILABLE(macos(12.3))
@interface AudioCaptureDelegate : NSObject <SCStreamDelegate, SCStreamOutput>
@property (nonatomic, assign) void* goContext;
@property (nonatomic, assign) int targetSampleRate;
@end

API_AVAILABLE(macos(12.3))
@implementation AudioCaptureDelegate

- (void)stream:(SCStream *)stream didOutputSampleBuffer:(CMSampleBufferRef)sampleBuffer ofType:(SCStreamOutputType)type {
    if (type != SCStreamOutputTypeAudio) {
        return;
    }

    // Get audio buffer
    CMBlockBufferRef blockBuffer = CMSampleBufferGetDataBuffer(sampleBuffer);
    if (blockBuffer == NULL) {
        return;
    }

    size_t length = 0;
    char* data = NULL;
    OSStatus status = CMBlockBufferGetDataPointer(blockBuffer, 0, NULL, &length, &data);
    if (status != kCMBlockBufferNoErr || data == NULL) {
        return;
    }

    // Get format description
    CMFormatDescriptionRef formatDesc = CMSampleBufferGetFormatDescription(sampleBuffer);
    const AudioStreamBasicDescription* asbd = CMAudioFormatDescriptionGetStreamBasicDescription(formatDesc);
    if (asbd == NULL) {
        return;
    }

    // Convert to float32 and resample if needed
    int numSamples = (int)(length / sizeof(float));
    float* floatData = (float*)data;

    // If stereo, convert to mono
    if (asbd->mChannelsPerFrame == 2) {
        int monoSamples = numSamples / 2;
        float* monoData = (float*)malloc(monoSamples * sizeof(float));
        for (int i = 0; i < monoSamples; i++) {
            monoData[i] = (floatData[i * 2] + floatData[i * 2 + 1]) / 2.0f;
        }

        // Resample to target sample rate if needed
        // ScreenCaptureKit typically outputs 48kHz, Whisper expects 16kHz
        if ((int)asbd->mSampleRate == 48000 && self.targetSampleRate == 16000) {
            int resampledCount = monoSamples / 3;
            float* resampledData = (float*)malloc(resampledCount * sizeof(float));
            for (int i = 0; i < resampledCount; i++) {
                resampledData[i] = monoData[i * 3];
            }
            goAudioCallback(self.goContext, resampledData, resampledCount);
            free(resampledData);
        } else {
            goAudioCallback(self.goContext, monoData, monoSamples);
        }
        free(monoData);
    } else {
        goAudioCallback(self.goContext, floatData, numSamples);
    }
}

- (void)stream:(SCStream *)stream didStopWithError:(NSError *)error {
    if (error) {
        NSLog(@"SCStream stopped with error: %@", error);
    }
}

@end

// Global state
static SCStream* currentStream API_AVAILABLE(macos(12.3)) = nil;
static AudioCaptureDelegate* currentDelegate API_AVAILABLE(macos(12.3)) = nil;

// Start audio capture with callback
int startAudioCaptureWithCallback(void* goContext, int targetSampleRate) {
    if (@available(macOS 12.3, *)) {
        dispatch_semaphore_t semaphore = dispatch_semaphore_create(0);
        __block int result = 0;

        dispatch_async(dispatch_get_main_queue(), ^{
            [SCShareableContent getShareableContentWithCompletionHandler:^(SCShareableContent * _Nullable content, NSError * _Nullable error) {
                if (error) {
                    NSLog(@"Failed to get shareable content: %@", error);
                    result = -1;
                    dispatch_semaphore_signal(semaphore);
                    return;
                }

                if (content.displays.count == 0) {
                    NSLog(@"No displays available");
                    result = -2;
                    dispatch_semaphore_signal(semaphore);
                    return;
                }

                // Use first display
                SCDisplay* display = content.displays[0];

                // Create content filter for the display
                SCContentFilter* filter = [[SCContentFilter alloc] initWithDisplay:display excludingApplications:@[] exceptingWindows:@[]];

                // Configure stream
                SCStreamConfiguration* config = [[SCStreamConfiguration alloc] init];
                config.capturesAudio = YES;
                config.excludesCurrentProcessAudio = NO;

                // Minimal video capture (we only need audio)
                config.width = 2;
                config.height = 2;
                config.minimumFrameInterval = CMTimeMake(1, 1); // 1 fps minimal

                // Audio configuration
                config.sampleRate = 48000; // ScreenCaptureKit uses 48kHz
                config.channelCount = 2;

                // Create delegate
                currentDelegate = [[AudioCaptureDelegate alloc] init];
                currentDelegate.goContext = goContext;
                currentDelegate.targetSampleRate = targetSampleRate;

                // Create stream
                currentStream = [[SCStream alloc] initWithFilter:filter configuration:config delegate:currentDelegate];

                NSError* addOutputError = nil;
                [currentStream addStreamOutput:currentDelegate type:SCStreamOutputTypeAudio sampleHandlerQueue:dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_HIGH, 0) error:&addOutputError];
                if (addOutputError) {
                    NSLog(@"Failed to add stream output: %@", addOutputError);
                    result = -3;
                    dispatch_semaphore_signal(semaphore);
                    return;
                }

                [currentStream startCaptureWithCompletionHandler:^(NSError * _Nullable startError) {
                    if (startError) {
                        NSLog(@"Failed to start capture: %@", startError);
                        result = -4;
                    } else {
                        result = 0;
                    }
                    dispatch_semaphore_signal(semaphore);
                }];
            }];
        });

        dispatch_semaphore_wait(semaphore, DISPATCH_TIME_FOREVER);
        return result;
    }
    return -100; // macOS version not supported
}

// Stop audio capture
void stopAudioCapture(void) {
    if (@available(macOS 12.3, *)) {
        if (currentStream != nil) {
            dispatch_semaphore_t semaphore = dispatch_semaphore_create(0);

            [currentStream stopCaptureWithCompletionHandler:^(NSError * _Nullable error) {
                if (error) {
                    NSLog(@"Failed to stop capture: %@", error);
                }
                dispatch_semaphore_signal(semaphore);
            }];

            dispatch_semaphore_wait(semaphore, dispatch_time(DISPATCH_TIME_NOW, 5 * NSEC_PER_SEC));
            currentStream = nil;
            currentDelegate = nil;
        }
    }
}

// Check if capturing
int isAudioCapturing(void) {
    if (@available(macOS 12.3, *)) {
        return currentStream != nil ? 1 : 0;
    }
    return 0;
}

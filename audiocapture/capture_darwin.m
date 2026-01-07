// capture_darwin.m - Objective-C implementation for ScreenCaptureKit audio capture

#import <ScreenCaptureKit/ScreenCaptureKit.h>
#import <CoreMedia/CoreMedia.h>
#import <Foundation/Foundation.h>
#include <stdlib.h>
#include <string.h>

// Forward declaration of Go callback
extern void goAudioCallback(float* samples, int count);

// Audio capture delegate
API_AVAILABLE(macos(12.3))
@interface AudioCaptureDelegate : NSObject <SCStreamDelegate, SCStreamOutput>
@property (nonatomic, assign) int targetSampleRate;
@end

API_AVAILABLE(macos(12.3))
@implementation AudioCaptureDelegate

- (void)stream:(SCStream *)stream didOutputSampleBuffer:(CMSampleBufferRef)sampleBuffer ofType:(SCStreamOutputType)type {
    if (type != SCStreamOutputTypeAudio) {
        return;
    }

    CMBlockBufferRef blockBuffer = CMSampleBufferGetDataBuffer(sampleBuffer);
    if (blockBuffer == NULL) {
        return;
    }

    size_t length = 0;
    char* data = NULL;
    if (CMBlockBufferGetDataPointer(blockBuffer, 0, NULL, &length, &data) != kCMBlockBufferNoErr || data == NULL) {
        return;
    }

    CMFormatDescriptionRef formatDesc = CMSampleBufferGetFormatDescription(sampleBuffer);
    const AudioStreamBasicDescription* asbd = CMAudioFormatDescriptionGetStreamBasicDescription(formatDesc);
    if (asbd == NULL) {
        return;
    }

    float* floatData = (float*)data;
    int numSamples = (int)(length / sizeof(float));
    int channels = (int)asbd->mChannelsPerFrame;

    // Stereo to mono + resample in one pass
    if (channels == 2 && (int)asbd->mSampleRate == 48000 && self.targetSampleRate == 16000) {
        // 48kHz stereo -> 16kHz mono: take every 3rd pair, average L+R
        int outCount = numSamples / 6;
        float* out = (float*)malloc(outCount * sizeof(float));
        for (int i = 0; i < outCount; i++) {
            int j = i * 6;
            out[i] = (floatData[j] + floatData[j + 1]) * 0.5f;
        }
        goAudioCallback(out, outCount);
        free(out);
    } else if (channels == 2 && (int)asbd->mSampleRate == 48000 && self.targetSampleRate == 48000) {
        // 48kHz stereo passthrough - no conversion needed
        goAudioCallback(floatData, numSamples);
    } else if (channels == 2) {
        // Stereo to mono only (other sample rates)
        int monoCount = numSamples / 2;
        float* out = (float*)malloc(monoCount * sizeof(float));
        for (int i = 0; i < monoCount; i++) {
            out[i] = (floatData[i * 2] + floatData[i * 2 + 1]) * 0.5f;
        }
        goAudioCallback(out, monoCount);
        free(out);
    } else {
        // Mono passthrough
        goAudioCallback(floatData, numSamples);
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

// Helper to set error string
static void setError(char** errOut, NSString* msg) {
    if (errOut != NULL) {
        const char* utf8 = [msg UTF8String];
        *errOut = strdup(utf8);
    }
}

// Start audio capture
int startAudioCapture(int targetSampleRate, char** errOut) {
    if (@available(macOS 12.3, *)) {
        dispatch_semaphore_t sem = dispatch_semaphore_create(0);
        __block int result = 0;
        __block NSString* errorMsg = nil;

        dispatch_async(dispatch_get_main_queue(), ^{
            [SCShareableContent getShareableContentWithCompletionHandler:^(SCShareableContent* content, NSError* error) {
                if (error) {
                    errorMsg = [NSString stringWithFormat:@"screen recording permission required: %@", error.localizedDescription];
                    result = -1;
                    dispatch_semaphore_signal(sem);
                    return;
                }

                if (content.displays.count == 0) {
                    errorMsg = @"no displays available";
                    result = -1;
                    dispatch_semaphore_signal(sem);
                    return;
                }

                SCDisplay* display = content.displays[0];
                SCContentFilter* filter = [[SCContentFilter alloc] initWithDisplay:display excludingApplications:@[] exceptingWindows:@[]];

                SCStreamConfiguration* config = [[SCStreamConfiguration alloc] init];
                config.capturesAudio = YES;
                config.excludesCurrentProcessAudio = NO;
                config.width = 2;
                config.height = 2;
                config.minimumFrameInterval = CMTimeMake(1, 1);
                config.sampleRate = 48000;
                config.channelCount = 2;

                currentDelegate = [[AudioCaptureDelegate alloc] init];
                currentDelegate.targetSampleRate = targetSampleRate;

                currentStream = [[SCStream alloc] initWithFilter:filter configuration:config delegate:currentDelegate];

                NSError* addErr = nil;
                [currentStream addStreamOutput:currentDelegate type:SCStreamOutputTypeAudio sampleHandlerQueue:dispatch_get_global_queue(QOS_CLASS_USER_INTERACTIVE, 0) error:&addErr];
                if (addErr) {
                    errorMsg = [NSString stringWithFormat:@"failed to add stream output: %@", addErr.localizedDescription];
                    result = -1;
                    dispatch_semaphore_signal(sem);
                    return;
                }

                [currentStream startCaptureWithCompletionHandler:^(NSError* startErr) {
                    if (startErr) {
                        errorMsg = [NSString stringWithFormat:@"failed to start capture: %@", startErr.localizedDescription];
                        result = -1;
                    }
                    dispatch_semaphore_signal(sem);
                }];
            }];
        });

        dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
        if (result != 0 && errorMsg) {
            setError(errOut, errorMsg);
        }
        return result;
    }
    setError(errOut, @"macOS 12.3 or later required");
    return -1;
}

// Stop audio capture
void stopAudioCapture(void) {
    if (@available(macOS 12.3, *)) {
        if (currentStream != nil) {
            dispatch_semaphore_t sem = dispatch_semaphore_create(0);
            [currentStream stopCaptureWithCompletionHandler:^(NSError* error) {
                dispatch_semaphore_signal(sem);
            }];
            dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, 5 * NSEC_PER_SEC));
            currentStream = nil;
            currentDelegate = nil;
        }
    }
}

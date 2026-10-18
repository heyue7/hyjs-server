// Read a selected screenshot locally and emit recognized text boxes without image data.
#import <Foundation/Foundation.h>
#import <Vision/Vision.h>
#include <signal.h>
#include <unistd.h>

static void ocrDeadline(int signalNumber) {
    (void)signalNumber;
    const char message[] = "Local screenshot OCR timed out\n";
    write(STDERR_FILENO, message, sizeof(message) - 1);
    _exit(124);
}

int main(int argc, const char *argv[]) {
    // Bound Vision model loading as well as recognition, even if the framework stalls.
    signal(SIGALRM, ocrDeadline);
    alarm(12);
    @autoreleasepool {
        if (argc < 2) {
            fprintf(stderr, "Usage: device_ocr IMAGE [TEXT_FILTER]\n");
            return 2;
        }
        NSString *filter = argc > 2 ? @(argv[2]) : @"";
        VNRecognizeTextRequest *request = [[VNRecognizeTextRequest alloc] init];
        request.recognitionLevel = VNRequestTextRecognitionLevelAccurate;
        // Use the Chinese-capable revision 2 engine on the CPU for local screenshots.
        request.revision = VNRecognizeTextRequestRevision2;
        request.usesCPUOnly = YES;
        request.recognitionLanguages = @[@"zh-Hans", @"en-US"];
        request.usesLanguageCorrection = NO;
        VNImageRequestHandler *handler = [[VNImageRequestHandler alloc]
            initWithURL:[NSURL fileURLWithPath:@(argv[1])] options:@{}];
        NSError *error = nil;
        if (![handler performRequests:@[request] error:&error]) {
            fprintf(stderr, "Local screenshot OCR failed\n");
            return 1;
        }
        NSMutableArray *entries = [NSMutableArray array];
        for (VNRecognizedTextObservation *item in request.results) {
            VNRecognizedText *candidate = [[item topCandidates:1] firstObject];
            if (!candidate || (filter.length && ![candidate.string containsString:filter])) continue;
            CGRect box = item.boundingBox;
            [entries addObject:@{@"text": candidate.string, @"confidence": @(candidate.confidence),
                                 @"x": @(CGRectGetMidX(box)), @"y": @(1 - CGRectGetMidY(box)),
                                 @"width": @(box.size.width), @"height": @(box.size.height)}];
        }
        NSData *data = [NSJSONSerialization dataWithJSONObject:entries options:0 error:&error];
        if (!data) return 1;
        fwrite(data.bytes, 1, data.length, stdout);
        fputc('\n', stdout);
        alarm(0);
    }
    return 0;
}

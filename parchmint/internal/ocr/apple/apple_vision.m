#import <Foundation/Foundation.h>
#import <Vision/Vision.h>
#import <CoreImage/CoreImage.h>

const char *performAppleVisionOCR(const void *imageBytes, size_t length, const char **langs, size_t langsCount) {
    @autoreleasepool {
        // Create NSData from the provided image bytes
        NSData *imageData = [NSData dataWithBytes:imageBytes length:length];
        if (!imageData) {
            return strdup("{\"error\": \"Failed to create NSData from image bytes.\"}");
        }

        // Create a CIImage from the NSData
        CIImage *ciImage = [CIImage imageWithData:imageData];
        if (!ciImage) {
            return strdup("{\"error\": \"Failed to create CIImage from NSData.\"}");
        }

        // Create a request handler
        VNImageRequestHandler *handler = [[VNImageRequestHandler alloc] initWithCIImage:ciImage options:@{}];

        // Create an array for languages and always include "en-US"
        NSMutableArray *languages = [NSMutableArray array];
//         [languages addObject:@"en-US"];

        // If an array of languages is provided, iterate and add them (avoiding duplicates)
        if (langs != NULL && langsCount > 0) {
            for (size_t i = 0; i < langsCount; i++) {
                NSString *specifiedLanguage = [NSString stringWithUTF8String:langs[i]];
                if (specifiedLanguage && ![specifiedLanguage isEqualToString:@"en-US"]) {
                    [languages addObject:specifiedLanguage];
                }
            }
        }

        // Set up the OCR request using the languages array
        VNRecognizeTextRequest *request = [[VNRecognizeTextRequest alloc] init];
        request.recognitionLevel = VNRequestTextRecognitionLevelAccurate;
        request.recognitionLanguages = languages;

        NSError *error = nil;
        [handler performRequests:@[request] error:&error];
        if (error) {
            NSString *errorJSON = [NSString stringWithFormat:@"{\"error\": \"%@\"}", error.localizedDescription];
            return strdup([errorJSON UTF8String]);
        }

        // Collect recognized text with granular coordinates
        NSMutableArray *results = [NSMutableArray array];
        for (VNRecognizedTextObservation *observation in request.results) {
            NSArray<VNRecognizedText *> *topCandidates = [observation topCandidates:1];
            if (topCandidates.count > 0) {
                VNRecognizedText *text = topCandidates.firstObject;

                // Split text into words, filtering out any empty strings
                NSArray *rawWords = [text.string componentsSeparatedByCharactersInSet:[NSCharacterSet whitespaceCharacterSet]];
                NSMutableArray *wordsArray = [NSMutableArray array];
                for (NSString *word in rawWords) {
                    if (word.length > 0) {
                        [wordsArray addObject:word];
                    }
                }

                // Calculate total characters in words (excluding spaces)
                NSInteger totalWordChars = 0;
                for (NSString *word in wordsArray) {
                    totalWordChars += word.length;
                }

                // Use proportional widths based on character count of each word
                NSMutableArray *words = [NSMutableArray array];
                CGFloat currentX = observation.boundingBox.origin.x;
                for (NSString *word in wordsArray) {
                    CGFloat wordFraction = (totalWordChars > 0) ? ((CGFloat)word.length / totalWordChars) : 0;
                    CGFloat wordWidth = observation.boundingBox.size.width * wordFraction;
                    CGRect wordBox = CGRectMake(
                        currentX,
                        observation.boundingBox.origin.y,
                        wordWidth,
                        observation.boundingBox.size.height
                    );
                    currentX += wordWidth; // Move to the next word position

                    NSDictionary *wordInfo = @{
                        @"text": word,
                        @"left": @(wordBox.origin.x),
                        @"top": @(1.0 - wordBox.origin.y - wordBox.size.height),
                        @"width": @(wordBox.size.width),
                        @"height": @(wordBox.size.height)
                    };

                    [words addObject:wordInfo];
                }

                // Include line bounding box information
                NSDictionary *lineDimensions = @{
                    @"left": @(observation.boundingBox.origin.x),
                    @"top": @(1.0 - observation.boundingBox.origin.y - observation.boundingBox.size.height),
                    @"width": @(observation.boundingBox.size.width),
                    @"height": @(observation.boundingBox.size.height)
                };

                // Add result for this observation
                NSDictionary *lineInfo = @{
                    @"text": text.string,
                    @"confidence": @(text.confidence),
                    @"rect": lineDimensions,
                    @"words": words
                };
                [results addObject:lineInfo];
            }
        }

        // Convert results to JSON
        NSError *jsonError = nil;
        NSData *jsonData = [NSJSONSerialization dataWithJSONObject:results options:NSJSONWritingPrettyPrinted error:&jsonError];
        if (jsonError) {
            return strdup("{\"error\": \"Failed to generate JSON.\"}");
        }

        return strdup([[NSString alloc] initWithData:jsonData encoding:NSUTF8StringEncoding].UTF8String);
    }
}

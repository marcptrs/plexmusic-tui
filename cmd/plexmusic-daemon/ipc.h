#import <Foundation/Foundation.h>

typedef void (^IPCMessageHandler)(NSDictionary *message);

@interface IPCServer : NSObject

@property (nonatomic, copy) IPCMessageHandler messageHandler;

- (BOOL)start;
- (void)stop;
- (void)sendCommand:(NSString *)command;
- (void)sendMessage:(NSDictionary *)message;

@end

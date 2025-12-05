#import "ipc.h"
#import <sys/socket.h>
#import <sys/un.h>
#import <unistd.h>

#define SOCKET_PATH "/tmp/plexmusic-daemon.sock"

@interface IPCServer ()
@property (nonatomic, assign) int serverSocket;
@property (nonatomic, assign) int clientSocket;
@property (nonatomic, strong) NSFileHandle *clientFileHandle;
@property (nonatomic, strong) dispatch_queue_t socketQueue;
@property (nonatomic, strong) NSLock *clientSocketLock;
@property (nonatomic, assign) BOOL running;
@end

@implementation IPCServer

- (instancetype)init {
    self = [super init];
    if (self) {
        _serverSocket = -1;
        _clientSocket = -1;
        _socketQueue = dispatch_queue_create("com.plexmusic.daemon.ipc", DISPATCH_QUEUE_SERIAL);
        _clientSocketLock = [[NSLock alloc] init];
    }
    return self;
}

- (BOOL)start {
    // Remove old socket file if it exists
    unlink(SOCKET_PATH);

    // Create Unix domain socket
    self.serverSocket = socket(AF_UNIX, SOCK_STREAM, 0);
    if (self.serverSocket < 0) {
        NSLog(@"[IPC] Failed to create socket: %s", strerror(errno));
        return NO;
    }

    struct sockaddr_un addr;
    memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    strncpy(addr.sun_path, SOCKET_PATH, sizeof(addr.sun_path) - 1);

    if (bind(self.serverSocket, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
        NSLog(@"[IPC] Failed to bind socket: %s", strerror(errno));
        close(self.serverSocket);
        return NO;
    }

    if (listen(self.serverSocket, 1) < 0) {
        NSLog(@"[IPC] Failed to listen on socket: %s", strerror(errno));
        close(self.serverSocket);
        return NO;
    }

    NSLog(@"[IPC] Server listening on %s", SOCKET_PATH);

    self.running = YES;

    // Accept connections in background
    dispatch_async(self.socketQueue, ^{
        [self acceptLoop];
    });

    return YES;
}

- (void)stop {
    self.running = NO;

    [self.clientSocketLock lock];
    if (self.clientSocket >= 0) {
        close(self.clientSocket);
        self.clientSocket = -1;
    }
    [self.clientSocketLock unlock];

    if (self.serverSocket >= 0) {
        close(self.serverSocket);
        self.serverSocket = -1;
    }

    unlink(SOCKET_PATH);
}

- (void)acceptLoop {
    while (self.running) {
        NSLog(@"[IPC] Waiting for client connection...");

        struct sockaddr_un clientAddr;
        socklen_t clientLen = sizeof(clientAddr);
        int client = accept(self.serverSocket, (struct sockaddr *)&clientAddr, &clientLen);

        if (client < 0) {
            if (self.running) {
                NSLog(@"[IPC] Accept failed: %s", strerror(errno));
            }
            continue;
        }

        NSLog(@"[IPC] Client connected");

        [self.clientSocketLock lock];
        // Close previous client if exists
        if (self.clientSocket >= 0) {
            close(self.clientSocket);
        }

        self.clientSocket = client;
        [self.clientSocketLock unlock];

        [self receiveMessages];
    }
}

- (void)receiveMessages {
    NSFileHandle *fileHandle = [[NSFileHandle alloc] initWithFileDescriptor:self.clientSocket closeOnDealloc:NO];

    while (self.running && self.clientSocket >= 0) {
        @autoreleasepool {
            // Read message length (4 bytes)
            NSData *lengthData = [fileHandle readDataOfLength:4];
            if (lengthData.length == 0) {
                NSLog(@"[IPC] Client disconnected");
                break;
            }

            if (lengthData.length < 4) {
                NSLog(@"[IPC] Invalid length data");
                break;
            }

            uint32_t messageLength;
            [lengthData getBytes:&messageLength length:4];
            messageLength = ntohl(messageLength);

            if (messageLength == 0 || messageLength > 1024 * 1024) { // Max 1MB
                NSLog(@"[IPC] Invalid message length: %u", messageLength);
                break;
            }

            // Read message content
            NSData *messageData = [fileHandle readDataOfLength:messageLength];
            if (messageData.length != messageLength) {
                NSLog(@"[IPC] Failed to read complete message");
                break;
            }

            // Parse JSON
            NSError *error = nil;
            NSDictionary *message = [NSJSONSerialization JSONObjectWithData:messageData options:0 error:&error];

            if (error || ![message isKindOfClass:[NSDictionary class]]) {
                NSLog(@"[IPC] Failed to parse JSON: %@", error);
                continue;
            }

            // Handle message on main thread
            dispatch_async(dispatch_get_main_queue(), ^{
                if (self.messageHandler) {
                    self.messageHandler(message);
                }
            });
        }
    }

    [self.clientSocketLock lock];
    if (self.clientSocket >= 0) {
        close(self.clientSocket);
        self.clientSocket = -1;
    }
    [self.clientSocketLock unlock];
}

- (void)sendCommand:(NSString *)command {
    NSDictionary *message = @{@"type": command};
    [self sendMessage:message];
}

- (void)sendMessage:(NSDictionary *)message {
    [self.clientSocketLock lock];

    if (self.clientSocket < 0) {
        NSLog(@"[IPC] No client connected, cannot send message");
        [self.clientSocketLock unlock];
        return;
    }

    NSError *error = nil;
    NSData *jsonData = [NSJSONSerialization dataWithJSONObject:message options:0 error:&error];

    if (error) {
        NSLog(@"[IPC] Failed to serialize message: %@", error);
        [self.clientSocketLock unlock];
        return;
    }

    uint32_t length = htonl((uint32_t)jsonData.length);
    NSMutableData *packet = [NSMutableData dataWithBytes:&length length:4];
    [packet appendData:jsonData];

    ssize_t sent = send(self.clientSocket, packet.bytes, packet.length, 0);
    if (sent < 0) {
        NSLog(@"[IPC] Failed to send message: %s", strerror(errno));
    } else {
        NSLog(@"[IPC] Sent message: %@", message[@"type"]);
    }

    [self.clientSocketLock unlock];
}

- (void)dealloc {
    [self stop];
}

@end

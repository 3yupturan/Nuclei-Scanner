

/**
 * SSHClient Class
 */
export class SSHClient {
    

    // Constructor of SSHClient
    constructor() {}
    /**
    * SetTimeout sets the timeout for the SSH connection in seconds
    */
    public SetTimeout(sec: number): void {
        return;
    }
    

    /**
    * Connect tries to connect to provided host and port
    * with provided username and password with ssh.
    * Returns state of connection and error. If error is not nil,
    * state will be false
    * @throws {Error} - if the operation fails
    */
    public Connect(host: string, port: number, username: string): boolean | null {
        return null;
    }
    

    /**
    * ConnectWithKey tries to connect to provided host and port
    * with provided username and private_key.
    * Returns state of connection and error. If error is not nil,
    * state will be false
    * @throws {Error} - if the operation fails
    */
    public ConnectWithKey(host: string, port: number, username: string): boolean | null {
        return null;
    }
    

    /**
    * ConnectSSHInfoMode tries to connect to provided host and port
    * with provided host and port
    * Returns HandshakeLog and error. If error is not nil,
    * state will be false
    * HandshakeLog is a struct that contains information about the
    * ssh connection
    * @throws {Error} - if the operation fails
    */
    public ConnectSSHInfoMode(host: string, port: number): HandshakeLog | null | null {
        return null;
    }
    

    /**
    * Run tries to open a new SSH session, then tries to execute
    * the provided command in said session
    * Returns string and error. If error is not nil,
    * state will be false
    * The string contains the command output
    * @throws {Error} - if the operation fails
    */
    public Run(cmd: string): string | null {
        return null;
    }
    

    /**
    * Close closes the SSH connection and destroys the client
    * Returns the success state and error. If error is not nil,
    * state will be false
    * @throws {Error} - if the operation fails
    */
    public Close(): boolean | null {
        return null;
    }
    

}



/**
 * Algorithms Interface
 */
export interface Algorithms {
    
    HostKey?: string,
    
    Kex?: string,
    
    W?: DirectionAlgorithms,
    
    R?: DirectionAlgorithms,
}



/**
 * DirectionAlgorithms Interface
 */
export interface DirectionAlgorithms {
    
    Compression?: string,
    
    Cipher?: string,
    
    MAC?: string,
}



/**
 * EndpointId Interface
 */
export interface EndpointId {
    
    Raw?: string,
    
    ProtoVersion?: string,
    
    SoftwareVersion?: string,
    
    Comment?: string,
}



/**
 * HandshakeLog Interface
 */
export interface HandshakeLog {
    
    Banner?: string,
    
    UserAuth?: string[],
    
    ServerID?: EndpointId,
    
    ClientID?: EndpointId,
    
    ServerKex?: KexInitMsg,
    
    ClientKex?: KexInitMsg,
    
    AlgorithmSelection?: Algorithms,
}



/**
 * KexInitMsg Interface
 */
export interface KexInitMsg {
    
    ServerHostKeyAlgos?: string[],
    
    MACsClientServer?: string[],
    
    LanguagesServerClient?: string[],
    
    KexAlgos?: string[],
    
    CiphersServerClient?: string[],
    
    LanguagesClientServer?: string[],
    
    FirstKexFollows?: boolean,
    
    Reserved?: number,
    
    /**
    * fixed size array of length: [16]
    */
    
    Cookie?: Uint8Array,
    
    CiphersClientServer?: string[],
    
    MACsServerClient?: string[],
    
    CompressionClientServer?: string[],
    
    CompressionServerClient?: string[],
}


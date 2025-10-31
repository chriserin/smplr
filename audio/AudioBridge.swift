import AVFoundation
import Foundation
import ScreenCaptureKit

// Global state for audio recorder and player
private var gSystemAudioRecorder: SystemAudioRecorder?
private var gAudioEngineManager: AudioEngineManager?
private var gCompletionCallback: (@convention(c) (Int32) -> Void)?

// Audio Engine Manager class
class AudioEngineManager {
    private let engine: AVAudioEngine
    private var players: [Int32: AVAudioPlayerNode] = [:]
    private var playerBuffers: [Int32: AVAudioPCMBuffer] = [:]
    private var timePitchUnits: [Int32: AVAudioUnitTimePitch] = [:]
    private var nextPlayerID: Int32 = 1

    init() {
        engine = AVAudioEngine()
    }

    func start() throws {
        if !engine.isRunning {
            try engine.start()
        }
    }

    func createPlayer(_ fileURL: URL) throws -> Int32 {
        let playerID = nextPlayerID
        nextPlayerID += 1

        // Load the audio file and buffer first to get the format
        let audioFile = try AVAudioFile(forReading: fileURL)
        let frameCount = AVAudioFrameCount(audioFile.length)
        let format = audioFile.processingFormat

        guard
            let buffer = AVAudioPCMBuffer(
                pcmFormat: format,
                frameCapacity: frameCount
            )
        else {
            throw NSError(
                domain: "AudioEngineManager", code: -2,
                userInfo: [NSLocalizedDescriptionKey: "Failed to create audio buffer"])
        }

        try audioFile.read(into: buffer)

        // Create and connect the player node with the actual audio format
        let playerNode = AVAudioPlayerNode()
        let timePitch = AVAudioUnitTimePitch()

        engine.attach(playerNode)
        engine.attach(timePitch)

        // Connect: player -> timePitch -> mixer
        engine.connect(playerNode, to: timePitch, format: format)
        engine.connect(timePitch, to: engine.mainMixerNode, format: format)

        players[playerID] = playerNode
        playerBuffers[playerID] = buffer
        timePitchUnits[playerID] = timePitch

        return playerID
    }

    func destroyPlayer(_ playerID: Int32) {
        guard let playerNode = players[playerID] else {
            print("Warning: Player ID \(playerID) not found")
            return
        }

        playerNode.stop()
        engine.disconnectNodeOutput(playerNode)
        engine.detach(playerNode)

        if let timePitch = timePitchUnits[playerID] {
            engine.disconnectNodeOutput(timePitch)
            engine.detach(timePitch)
        }

        players.removeValue(forKey: playerID)
        playerBuffers.removeValue(forKey: playerID)
        timePitchUnits.removeValue(forKey: playerID)
        print("Destroyed player with ID: \(playerID)")
    }

    func playFile(_ playerID: Int32, _ fileURL: URL, cents: Float) throws {
        guard let playerNode = players[playerID] else {
            throw NSError(
                domain: "AudioEngineManager", code: -1,
                userInfo: [NSLocalizedDescriptionKey: "Player ID \(playerID) not found"])
        }

        // Set pitch shift
        if let timePitch = timePitchUnits[playerID] {
            timePitch.pitch = cents
            timePitch.overlap = 8.0
        }

        // If buffer is loaded, use it; otherwise fall back to file
        if let buffer = playerBuffers[playerID] {
            playerNode.stop()
            playerNode.scheduleBuffer(buffer, at: nil) {
                // Call completion callback when playback finishes
                if let callback = gCompletionCallback {
                    callback(playerID)
                }
            }
            print("Latency " + String(playerNode.latency))
            playerNode.play()
        }
    }

    func playRegion(
        _ playerID: Int32, _ fileURL: URL, startFrame: Int32, endFrame: Int32, cents: Float
    ) throws {
        guard let playerNode = players[playerID] else {
            throw NSError(
                domain: "AudioEngineManager", code: -1,
                userInfo: [NSLocalizedDescriptionKey: "Player ID \(playerID) not found"])
        }

        // Set pitch shift
        if let timePitch = timePitchUnits[playerID] {
            timePitch.pitch = cents
        }

        // If buffer is loaded, create a segment buffer; otherwise use file
        if let sourceBuffer = playerBuffers[playerID] {
            let start = Int(startFrame)
            let end = Int(endFrame)
            let frameCount = end - start

            guard start >= 0 && end <= Int(sourceBuffer.frameLength) && frameCount > 0 else {
                throw NSError(
                    domain: "AudioEngineManager", code: -3,
                    userInfo: [NSLocalizedDescriptionKey: "Invalid frame range"])
            }

            guard
                let segmentBuffer = AVAudioPCMBuffer(
                    pcmFormat: sourceBuffer.format,
                    frameCapacity: AVAudioFrameCount(frameCount)
                )
            else {
                throw NSError(
                    domain: "AudioEngineManager", code: -2,
                    userInfo: [NSLocalizedDescriptionKey: "Failed to create segment buffer"])
            }

            // Copy the region from source buffer to segment buffer
            let channelCount = Int(sourceBuffer.format.channelCount)
            for channel in 0..<channelCount {
                let sourcePtr = sourceBuffer.floatChannelData![channel]
                let destPtr = segmentBuffer.floatChannelData![channel]
                memcpy(
                    destPtr, sourcePtr.advanced(by: start), frameCount * MemoryLayout<Float>.stride)
            }
            segmentBuffer.frameLength = AVAudioFrameCount(frameCount)

            playerNode.stop()
            playerNode.scheduleBuffer(segmentBuffer, at: nil) {
                // Call completion callback when playback finishes
                if let callback = gCompletionCallback {
                    callback(playerID)
                }
            }
            playerNode.play()
        }
    }
}

// System audio recorder class using ScreenCaptureKit
class SystemAudioRecorder: NSObject, SCStreamDelegate, SCStreamOutput {
    private var stream: SCStream?
    private var assetWriter: AVAssetWriter?
    private var assetWriterInput: AVAssetWriterInput?
    private let outputURL: URL
    private var isRecording = false

    init(outputPath: String) {
        self.outputURL = URL(fileURLWithPath: outputPath)
        super.init()
    }

    func startRecording() async throws {
        // Get available content
        let content = try await SCShareableContent.current

        // Get the main display
        guard let display = content.displays.first else {
            throw NSError(
                domain: "AudioRecorder", code: -1,
                userInfo: [NSLocalizedDescriptionKey: "No display found"])
        }

        // Get all applications except ourselves
        let excludedApps = content.applications.filter { app in
            app.bundleIdentifier.contains("bubbletea-poc")
        }

        // Create filter
        let filter = SCContentFilter(
            display: display, excludingApplications: excludedApps, exceptingWindows: [])

        // Configure stream settings
        let config = SCStreamConfiguration()
        config.capturesAudio = true
        config.excludesCurrentProcessAudio = true
        config.sampleRate = 48000
        config.channelCount = 2

        // Disable video capture
        config.width = 1
        config.height = 1
        config.pixelFormat = kCVPixelFormatType_32BGRA
        config.minimumFrameInterval = CMTime(value: 1, timescale: 1)
        config.queueDepth = 5

        // Create and start stream
        stream = SCStream(filter: filter, configuration: config, delegate: self)

        guard let stream = stream else {
            throw NSError(
                domain: "AudioRecorder", code: -2,
                userInfo: [NSLocalizedDescriptionKey: "Failed to create stream"])
        }

        // Add audio output
        try stream.addStreamOutput(
            self, type: .audio, sampleHandlerQueue: DispatchQueue(label: "audio.capture.queue"))

        // Start capture
        try await stream.startCapture()

        isRecording = true
    }

    func stopRecording() async {
        guard isRecording else { return }

        if let stream = stream {
            try? await stream.stopCapture()
        }

        stream = nil

        // Finish writing
        assetWriterInput?.markAsFinished()
        if let writer = assetWriter {
            await writer.finishWriting()
        }
        assetWriter = nil
        assetWriterInput = nil

        isRecording = false
    }

    // SCStreamOutput protocol method
    func stream(
        _ stream: SCStream, didOutputSampleBuffer sampleBuffer: CMSampleBuffer,
        of type: SCStreamOutputType
    ) {
        guard type == .audio else { return }

        let numSamples = CMSampleBufferGetNumSamples(sampleBuffer)
        guard numSamples > 0 else { return }

        // Initialize asset writer on first buffer
        if assetWriter == nil {
            do {
                guard let formatDescription = CMSampleBufferGetFormatDescription(sampleBuffer)
                else {
                    print("Failed to get audio format description")
                    return
                }

                // Remove existing file if it exists
                if FileManager.default.fileExists(atPath: outputURL.path) {
                    try? FileManager.default.removeItem(at: outputURL)
                }

                // Create asset writer
                assetWriter = try AVAssetWriter(outputURL: outputURL, fileType: .wav)

                // Get audio format details
                guard
                    let streamBasicDescription = CMAudioFormatDescriptionGetStreamBasicDescription(
                        formatDescription)
                else {
                    print("Failed to get stream basic description")
                    return
                }

                let audioSettings: [String: Any] = [
                    AVFormatIDKey: Int(kAudioFormatLinearPCM),
                    AVSampleRateKey: streamBasicDescription.pointee.mSampleRate,
                    AVNumberOfChannelsKey: Int(streamBasicDescription.pointee.mChannelsPerFrame),
                    AVLinearPCMBitDepthKey: 16,
                    AVLinearPCMIsFloatKey: false,
                    AVLinearPCMIsBigEndianKey: false,
                    AVLinearPCMIsNonInterleaved: false,
                ]

                let writerInput = AVAssetWriterInput(
                    mediaType: .audio, outputSettings: audioSettings)
                writerInput.expectsMediaDataInRealTime = true

                guard let writer = assetWriter else { return }

                if writer.canAdd(writerInput) {
                    writer.add(writerInput)
                    assetWriterInput = writerInput
                } else {
                    print("Cannot add audio input to writer")
                    return
                }

                writer.startWriting()

                if writer.status == .failed {
                    print("Writer failed: \(writer.error?.localizedDescription ?? "unknown")")
                    return
                }

                writer.startSession(
                    atSourceTime: CMSampleBufferGetPresentationTimeStamp(sampleBuffer))
            } catch {
                print("Error creating asset writer: \(error)")
                return
            }
        }

        // Write the sample buffer
        guard let input = assetWriterInput, input.isReadyForMoreMediaData else { return }

        if !input.append(sampleBuffer) {
            if let writer = assetWriter {
                print("Failed to append sample buffer. Status: \(writer.status.rawValue)")
            }
        }
    }

    // SCStreamDelegate method
    func stream(_ stream: SCStream, didStopWithError error: Error) {
        print("Stream stopped with error: \(error.localizedDescription)")
    }
}

// MARK: - C-callable functions

@_cdecl("SwiftAudio_init")
public func SwiftAudio_init() -> Int32 {
    let manager = AudioEngineManager()
    gAudioEngineManager = manager
    return 0
}

@_cdecl("SwiftAudio_start")
public func SwiftAudio_start() -> Int32 {
    guard let manager = gAudioEngineManager else {
        print("Error: Audio engine not initialized. Call Init() first.")
        return 1
    }

    do {
        try manager.start()
        return 0
    } catch {
        print("Error starting audio engine: \(error)")
        return 1
    }
}

@_cdecl("SwiftAudio_setCompletionCallback")
public func SwiftAudio_setCompletionCallback(_ callback: @escaping @convention(c) (Int32) -> Void) {
    gCompletionCallback = callback
}

@_cdecl("SwiftAudio_createPlayer")
public func SwiftAudio_createPlayer(_ filename: UnsafePointer<CChar>) -> Int32 {
    let filenameStr = String(cString: filename)
    let fileURL = URL(fileURLWithPath: filenameStr)

    guard let manager = gAudioEngineManager else {
        print("Error: Audio engine not initialized. Call Init() first.")
        return -1
    }

    do {
        return try manager.createPlayer(fileURL)
    } catch let error as NSError {
        // Check if this is the WAVE_FORMAT_EXTENSIBLE format issue
        if error.domain == "com.apple.coreaudio.avfaudio" && error.code == 1_685_348_671 {
            print("Error: WAVE_FORMAT_EXTENSIBLE (0xFFFE) format is not supported by AVFoundation")
            print("Please convert the WAV file to standard PCM format (format code 1)")
        } else {
            print("Error creating player: \(error)")
        }
        return -1
    }
}

@_cdecl("SwiftAudio_destroyPlayer")
public func SwiftAudio_destroyPlayer(_ playerID: Int32) -> Int32 {
    guard let manager = gAudioEngineManager else {
        print("Error: Audio engine not initialized.")
        return 1
    }

    manager.destroyPlayer(playerID)
    return 0
}

@_cdecl("SwiftAudio_record")
public func SwiftAudio_record(_ filename: UnsafePointer<CChar>) -> Int32 {
    let filenameStr = String(cString: filename)

    let recorder = SystemAudioRecorder(outputPath: filenameStr)
    gSystemAudioRecorder = recorder

    Task {
        do {
            try await recorder.startRecording()
        } catch {
            print("Error starting system audio recording: \(error)")
        }
    }

    return 0
}

@_cdecl("SwiftAudio_stopRecording")
public func SwiftAudio_stopRecording() -> Int32 {
    guard let recorder = gSystemAudioRecorder else {
        return 1
    }

    let semaphore = DispatchSemaphore(value: 0)

    Task {
        await recorder.stopRecording()
        semaphore.signal()
    }

    semaphore.wait()
    gSystemAudioRecorder = nil
    return 0
}

@_cdecl("SwiftAudio_playFile")
public func SwiftAudio_playFile(_ playerID: Int32, _ filename: UnsafePointer<CChar>, _ cents: Float)
    -> Int32
{
    let filenameStr = String(cString: filename)
    let fileURL = URL(fileURLWithPath: filenameStr)

    guard let manager = gAudioEngineManager else {
        print("Error: Audio engine not initialized. Call Init() first.")
        return 1
    }

    do {
        try manager.playFile(playerID, fileURL, cents: cents)
        return 0
    } catch let error as NSError {
        // Check if this is the WAVE_FORMAT_EXTENSIBLE format issue
        if error.domain == "com.apple.coreaudio.avfaudio" && error.code == 1_685_348_671 {
            print("Error: WAVE_FORMAT_EXTENSIBLE (0xFFFE) format is not supported by AVFoundation")
            print("Please convert the WAV file to standard PCM format (format code 1)")
        } else {
            print("Error playing file: \(error)")
        }
        return 1
    }
}

@_cdecl("SwiftAudio_playRegion")
public func SwiftAudio_playRegion(
    _ playerID: Int32, _ filename: UnsafePointer<CChar>, _ startFrame: Int32, _ endFrame: Int32,
    _ cents: Float
) -> Int32 {
    let filenameStr = String(cString: filename)
    let fileURL = URL(fileURLWithPath: filenameStr)

    guard let manager = gAudioEngineManager else {
        print("Error: Audio engine not initialized. Call Init() first.")
        return 1
    }

    do {
        try manager.playRegion(
            playerID, fileURL, startFrame: startFrame, endFrame: endFrame, cents: cents)
        return 0
    } catch {
        print("Error playing region: \(error)")
        return 1
    }
}

@_cdecl("SwiftAudio_trimFile")
public func SwiftAudio_trimFile(
    _ filename: UnsafePointer<CChar>, _ startFrame: Int32, _ endFrame: Int32
) -> Int32 {
    let filenameStr = String(cString: filename)
    let fileURL = URL(fileURLWithPath: filenameStr)

    do {
        let audioFile = try AVAudioFile(forReading: fileURL)
        let processingFormat = audioFile.processingFormat
        let frameCapacity = AVAudioFrameCount(endFrame - startFrame)

        guard
            let buffer = AVAudioPCMBuffer(pcmFormat: processingFormat, frameCapacity: frameCapacity)
        else {
            print("Error: Failed to create audio buffer")
            return 1
        }

        // Seek to start frame
        audioFile.framePosition = AVAudioFramePosition(startFrame)

        // Read frames
        try audioFile.read(into: buffer, frameCount: frameCapacity)

        // Write to temporary file
        let tempURL = fileURL.deletingLastPathComponent().appendingPathComponent(
            "temp_\(UUID().uuidString).wav")
        let outputFile = try AVAudioFile(
            forWriting: tempURL, settings: audioFile.fileFormat.settings)
        try outputFile.write(from: buffer)

        // Replace original file
        let fileManager = FileManager.default
        try fileManager.removeItem(at: fileURL)
        try fileManager.moveItem(at: tempURL, to: fileURL)

        return 0
    } catch {
        print("Error trimming file: \(error)")
        return 1
    }
}

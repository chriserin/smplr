import AVFoundation
import CoreAudio
import Foundation
import ScreenCaptureKit

// Global state for audio recorder and player
private var gSystemAudioRecorder: SystemAudioRecorder?
private var gAudioEngineManager: AudioEngineManager?
private var gCompletionCallback: (@convention(c) (Int32) -> Void)?
private var gDecibelCallback: (@convention(c) (Float) -> Void)?

// Audio Engine Manager class
class AudioEngineManager {
    private let engine: AVAudioEngine
    private var players: [Int32: AVAudioPlayerNode] = [:]
    private var playerBuffers: [Int32: AVAudioPCMBuffer] = [:]
    private var nextPlayerID: Int32 = 1
    private var deviceID: AudioDeviceID?

    init() {
        engine = AVAudioEngine()
    }

    // Helper function to find audio device by name
    private static func findDeviceByName(_ name: String) -> AudioDeviceID? {
        var propertySize: UInt32 = 0
        var propertyAddress = AudioObjectPropertyAddress(
            mSelector: kAudioHardwarePropertyDevices,
            mScope: kAudioObjectPropertyScopeGlobal,
            mElement: kAudioObjectPropertyElementMain
        )

        var status = AudioObjectGetPropertyDataSize(
            AudioObjectID(kAudioObjectSystemObject),
            &propertyAddress,
            0,
            nil,
            &propertySize
        )

        guard status == noErr else { return nil }

        let deviceCount = Int(propertySize) / MemoryLayout<AudioDeviceID>.size
        var deviceIDs = [AudioDeviceID](repeating: 0, count: deviceCount)

        status = AudioObjectGetPropertyData(
            AudioObjectID(kAudioObjectSystemObject),
            &propertyAddress,
            0,
            nil,
            &propertySize,
            &deviceIDs
        )

        guard status == noErr else { return nil }

        // Search for device with matching name
        for deviceID in deviceIDs {
            var namePropertyAddress = AudioObjectPropertyAddress(
                mSelector: kAudioObjectPropertyName,
                mScope: kAudioObjectPropertyScopeGlobal,
                mElement: kAudioObjectPropertyElementMain
            )

            var namePropertySize: UInt32 = UInt32(MemoryLayout<CFString?>.size)
            var deviceName: Unmanaged<CFString>?

            status = AudioObjectGetPropertyData(
                deviceID,
                &namePropertyAddress,
                0,
                nil,
                &namePropertySize,
                &deviceName
            )

            if status == noErr, let devName = deviceName?.takeUnretainedValue() as String? {
                if devName == name {
                    return deviceID
                }
            }
        }

        return nil
    }

    func start(deviceName: String) throws {
        // Look up and configure output device if specified
        if !deviceName.isEmpty {
            if let deviceID = AudioEngineManager.findDeviceByName(deviceName) {
                let outputNode = engine.outputNode
                let audioUnit = outputNode.audioUnit

                var deviceIDCopy = deviceID
                let status = AudioUnitSetProperty(
                    audioUnit!,
                    kAudioOutputUnitProperty_CurrentDevice,
                    kAudioUnitScope_Global,
                    0,
                    &deviceIDCopy,
                    UInt32(MemoryLayout<AudioDeviceID>.size)
                )

                if status != noErr {
                    print("Warning: Failed to set audio output device (error: \(status))")
                }
            } else {
                print(
                    "Warning: Could not find audio device with name '\(deviceName)', using default device"
                )
            }
        }

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

        // Create and connect the player node directly to mixer (no real-time pitch shifting)
        let playerNode = AVAudioPlayerNode()

        engine.attach(playerNode)

        // Connect: player -> mixer (direct, low latency)
        engine.connect(playerNode, to: engine.mainMixerNode, format: format)

        players[playerID] = playerNode
        playerBuffers[playerID] = buffer

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

        players.removeValue(forKey: playerID)
        playerBuffers.removeValue(forKey: playerID)
    }

    func stopPlayer(_ playerID: Int32) {
        guard let playerNode = players[playerID] else {
            print("Warning: Player ID \(playerID) not found")
            return
        }

        playerNode.stop()
    }

    func playFile(_ playerID: Int32, _ fileURL: URL, cents: Float) throws {
        guard let playerNode = players[playerID] else {
            print("Error: Player ID \(playerID) not found")
            throw NSError(
                domain: "AudioEngineManager", code: -1,
                userInfo: [NSLocalizedDescriptionKey: "Player ID \(playerID) not found"])
        }

        // No real-time pitch shifting - files are pre-rendered at the correct pitch
        // cents parameter is ignored (should be 0)

        // If buffer is loaded, use it; otherwise fall back to file
        if let buffer = playerBuffers[playerID] {
            playerNode.stop()
            playerNode.scheduleBuffer(buffer, at: nil) {
                // Call completion callback when playback finishes
                if let callback = gCompletionCallback {
                    callback(playerID)
                }
            }
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

        // No real-time pitch shifting - files are pre-rendered at the correct pitch
        // cents parameter is ignored (should be 0)

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
            app.bundleIdentifier.contains("smplr")
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

    // Calculate decibel level from audio buffer
    private func calculateDecibels(from sampleBuffer: CMSampleBuffer) -> Float {
        guard let blockBuffer = CMSampleBufferGetDataBuffer(sampleBuffer) else {
            return -160.0
        }

        var length: Int = 0
        var dataPointer: UnsafeMutablePointer<Int8>?

        guard
            CMBlockBufferGetDataPointer(
                blockBuffer, atOffset: 0, lengthAtOffsetOut: nil,
                totalLengthOut: &length, dataPointerOut: &dataPointer) == noErr,
            let data = dataPointer
        else {
            return -160.0
        }

        // Calculate samples (Float format from ScreenCaptureKit)
        let bytesPerSample = MemoryLayout<Float>.size
        let totalSamples = length / bytesPerSample

        // Process as Float samples (interleaved)
        let floatData = data.withMemoryRebound(to: Float.self, capacity: totalSamples) { $0 }

        // Calculate RMS across all channels
        var sum: Float = 0.0
        for i in 0..<totalSamples {
            let sample = floatData[i]
            sum += sample * sample
        }

        let rms = sqrt(sum / Float(totalSamples))
        let db = 20.0 * log10(max(rms, 0.00001))

        return db
    }

    // SCStreamOutput protocol method
    func stream(
        _ stream: SCStream, didOutputSampleBuffer sampleBuffer: CMSampleBuffer,
        of type: SCStreamOutputType
    ) {
        guard type == .audio else { return }

        let numSamples = CMSampleBufferGetNumSamples(sampleBuffer)
        guard numSamples > 0 else { return }

        // Calculate and send decibel level
        if let decibelCallback = gDecibelCallback {
            let db = calculateDecibels(from: sampleBuffer)
            decibelCallback(db)
        }

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
                    print("ERROR: Cannot add audio input to writer")
                    return
                }

                writer.startWriting()

                if writer.status == .failed {
                    print(
                        "ERROR: Writer failed: \(writer.error?.localizedDescription ?? "unknown")")
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
                print("ERROR: Failed to append sample buffer. Status: \(writer.status.rawValue)")
            }
        }
    }

    // SCStreamDelegate method
    func stream(_ stream: SCStream, didStopWithError error: Error) {
        print("ERROR: Stream stopped with error: \(error.localizedDescription)")
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
public func SwiftAudio_start(_ deviceName: UnsafePointer<CChar>?) -> Int32 {
    guard let manager = gAudioEngineManager else {
        print("Error: Audio engine not initialized. Call Init() first.")
        return 1
    }

    let deviceNameStr: String
    if let deviceName = deviceName {
        deviceNameStr = String(cString: deviceName)
    } else {
        deviceNameStr = ""
    }

    do {
        try manager.start(deviceName: deviceNameStr)
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

@_cdecl("SwiftAudio_setDecibelCallback")
public func SwiftAudio_setDecibelCallback(_ callback: @escaping @convention(c) (Float) -> Void) {
    gDecibelCallback = callback
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

@_cdecl("SwiftAudio_stopPlayer")
public func SwiftAudio_stopPlayer(_ playerID: Int32) -> Int32 {
    guard let manager = gAudioEngineManager else {
        print("Error: Audio engine not initialized.")
        return 1
    }

    manager.stopPlayer(playerID)
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

@_cdecl("SwiftAudio_renderPitchedFile")
public func SwiftAudio_renderPitchedFile(
    _ sourceFilename: UnsafePointer<CChar>, _ targetFilename: UnsafePointer<CChar>, _ cents: Float
) -> Int32 {
    let sourceStr = String(cString: sourceFilename)
    let targetStr = String(cString: targetFilename)
    let sourceURL = URL(fileURLWithPath: sourceStr)
    let targetURL = URL(fileURLWithPath: targetStr)

    do {
        // Load source audio file
        let sourceFile = try AVAudioFile(forReading: sourceURL)
        let sampleRate = sourceFile.processingFormat.sampleRate
        let channels = sourceFile.processingFormat.channelCount
        let sourceLength = sourceFile.length

        // Read source buffer
        guard
            let sourceBuffer = AVAudioPCMBuffer(
                pcmFormat: sourceFile.processingFormat,
                frameCapacity: AVAudioFrameCount(sourceLength)
            )
        else {
            print("Error: Failed to create source buffer")
            return 1
        }
        try sourceFile.read(into: sourceBuffer)
        sourceBuffer.frameLength = AVAudioFrameCount(sourceLength)

        // Calculate pitch ratio from semitones
        let semitones = Double(cents) / 100.0
        let pitchRatio = pow(2.0, semitones / 12.0)

        // Create Rubberband stretcher using C API
        let options: RubberBandOptions = Int32(
            RubberBandOptionProcessOffline.rawValue
                | RubberBandOptionPitchHighQuality.rawValue
                | RubberBandOptionChannelsApart.rawValue
                | RubberBandOptionEngineFiner.rawValue
        )

        let rb = rubberband_new(
            UInt32(sampleRate),
            UInt32(channels),
            options,
            1.0,  // Time ratio (no time stretching)
            pitchRatio
        )

        guard rb != nil else {
            print("Error: Failed to create Rubberband stretcher")
            return 1
        }
        defer { rubberband_delete(rb) }

        // Set expected input duration and max process size
        rubberband_set_expected_input_duration(rb, UInt32(sourceLength))
        rubberband_set_max_process_size(rb, UInt32(sourceLength))

        // Prepare input pointers
        var inputPtrs = [UnsafePointer<Float>?](repeating: nil, count: Int(channels))
        for i in 0..<Int(channels) {
            inputPtrs[i] = UnsafePointer(sourceBuffer.floatChannelData![i])
        }

        // Study and process (offline mode)
        inputPtrs.withUnsafeBufferPointer { ptrsBuffer in
            rubberband_study(rb, ptrsBuffer.baseAddress, UInt32(sourceLength), 1)
            rubberband_process(rb, ptrsBuffer.baseAddress, UInt32(sourceLength), 1)
        }

        // Get output frame count
        let outputFrames = Int(rubberband_available(rb))

        guard
            let outputBuffer = AVAudioPCMBuffer(
                pcmFormat: sourceFile.processingFormat,
                frameCapacity: AVAudioFrameCount(outputFrames)
            )
        else {
            print("Error: Failed to create output buffer")
            return 1
        }

        // Retrieve processed audio
        var outputPtrs = [UnsafeMutablePointer<Float>?](repeating: nil, count: Int(channels))
        for i in 0..<Int(channels) {
            outputPtrs[i] = outputBuffer.floatChannelData![i]
        }

        let retrieved = outputPtrs.withUnsafeMutableBufferPointer { ptrsBuffer in
            rubberband_retrieve(rb, ptrsBuffer.baseAddress, UInt32(outputFrames))
        }

        outputBuffer.frameLength = AVAudioFrameCount(retrieved)

        // Remove target file if it exists
        if FileManager.default.fileExists(atPath: targetURL.path) {
            try FileManager.default.removeItem(at: targetURL)
        }

        // Write to target file
        let outputFile = try AVAudioFile(
            forWriting: targetURL,
            settings: sourceFile.fileFormat.settings,
            commonFormat: .pcmFormatFloat32,
            interleaved: false
        )
        try outputFile.write(from: outputBuffer)

        return 0

    } catch {
        print("Error rendering pitched file: \(error)")
        return 1
    }
}

@_cdecl("SwiftAudio_getAudioDevices")
public func SwiftAudio_getAudioDevices() -> UnsafeMutablePointer<CChar>? {
    var result = ""

    #if os(macOS)
        // Get all audio devices
        var propertySize: UInt32 = 0
        var propertyAddress = AudioObjectPropertyAddress(
            mSelector: kAudioHardwarePropertyDevices,
            mScope: kAudioObjectPropertyScopeGlobal,
            mElement: kAudioObjectPropertyElementMain
        )

        var status = AudioObjectGetPropertyDataSize(
            AudioObjectID(kAudioObjectSystemObject),
            &propertyAddress,
            0,
            nil,
            &propertySize
        )

        guard status == noErr else {
            return strdup("")
        }

        let deviceCount = Int(propertySize) / MemoryLayout<AudioDeviceID>.size
        var deviceIDs = [AudioDeviceID](repeating: 0, count: deviceCount)

        status = AudioObjectGetPropertyData(
            AudioObjectID(kAudioObjectSystemObject),
            &propertyAddress,
            0,
            nil,
            &propertySize,
            &deviceIDs
        )

        guard status == noErr else {
            return strdup("")
        }

        // Filter for output devices and get their names
        for deviceID in deviceIDs {
            // Check if device has output streams
            var streamPropertyAddress = AudioObjectPropertyAddress(
                mSelector: kAudioDevicePropertyStreams,
                mScope: kAudioDevicePropertyScopeOutput,
                mElement: kAudioObjectPropertyElementMain
            )

            var streamPropertySize: UInt32 = 0
            status = AudioObjectGetPropertyDataSize(
                deviceID,
                &streamPropertyAddress,
                0,
                nil,
                &streamPropertySize
            )

            // Skip if no output streams
            guard status == noErr && streamPropertySize > 0 else {
                continue
            }

            // Get device name
            var namePropertyAddress = AudioObjectPropertyAddress(
                mSelector: kAudioObjectPropertyName,
                mScope: kAudioObjectPropertyScopeGlobal,
                mElement: kAudioObjectPropertyElementMain
            )

            var namePropertySize: UInt32 = UInt32(MemoryLayout<CFString?>.size)
            var deviceName: Unmanaged<CFString>?

            status = AudioObjectGetPropertyData(
                deviceID,
                &namePropertyAddress,
                0,
                nil,
                &namePropertySize,
                &deviceName
            )

            if status == noErr, let name = deviceName?.takeUnretainedValue() as String? {
                result += "\(deviceID)|\(name)\n"
            }
        }
    #endif

    return strdup(result)
}

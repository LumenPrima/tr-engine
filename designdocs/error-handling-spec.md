# Error Handling Specification

## Overview

This document defines the standardized approach to error handling across the TR-ENGINE system. The goal is to establish a consistent, informative, and maintainable error handling strategy that serves both users and developers while facilitating error tracking and analysis.

## Core Error Structure

Every error in the system follows a standardized format that captures both user-facing information and technical details:

```javascript
{
  status: "error",                    // Fixed string indicating error response
  timestamp: 1706543412,             // Unix timestamp (internal)
  error: {
    code: "ERROR_TYPE_CODE",         // Machine-readable error code
    message: "Human readable message", // User-facing message
    details: {                       // Optional detailed error info
      component: "audio_processor",   // System component where error occurred
      trace: "Error stack trace",    // Development/debug information
      validation: [{                 // For validation errors
        field: "frequency",
        error: "must be a number"
      }]
    },
    context: {                       // Request-specific context
      callId: "9131_1737430014",    // Relevant IDs or references
      request: "/api/v1/audio/...",  // Original request information
      params: {}                     // Request parameters
    }
  }
}
```

## Error Handler Implementation

The ErrorHandler service serves as the central point for error processing:

```javascript
class ErrorHandler {
  /**
   * Converts any error into our standard format
   * @param {Error} error - The original error
   * @param {Object} context - Additional context about the error
   * @returns {Object} Standardized error object
   */
  static formatError(error, context = {}) {
    const standardError = {
      status: "error",
      timestamp: Math.floor(Date.now() / 1000),
      error: {
        code: this.determineErrorCode(error),
        message: this.createUserMessage(error),
        details: {},
        context
      }
    };

    // Include stack traces in development environments
    if (process.env.NODE_ENV !== 'production') {
      standardError.error.details.trace = error.stack;
    }

    // Add validation errors when present
    if (error.validation) {
      standardError.error.details.validation = error.validation;
    }

    return standardError;
  }

  /**
   * Formats error appropriately for different interfaces
   * @param {Error} error - The original error
   * @param {string} interface - The interface type ('rest' or 'websocket')
   * @returns {Object} Interface-appropriate error format
   */
  static formatForInterface(error, interface = 'rest') {
    const standardError = this.formatError(error);
    
    switch(interface) {
      case 'websocket':
        return {
          type: 'error',
          timestamp: new Date(standardError.timestamp * 1000).toISOString(),
          error: standardError.error.message,
          data: {
            code: standardError.error.code,
            details: standardError.error.details
          }
        };
      
      case 'rest':
      default:
        return {
          ...standardError,
          timestamp: new Date(standardError.timestamp * 1000).toISOString()
        };
    }
  }

  /**
   * Handles error logging and storage
   * @param {Error} error - The original error
   * @param {Object} context - Additional context
   * @returns {Promise<Object>} Processed error
   */
  static async handleError(error, context = {}) {
    const standardError = this.formatError(error, context);
    
    // Log all errors
    logger.error('Error occurred', standardError);

    // Store significant errors for analysis
    if (this.shouldStoreError(error)) {
      await this.storeError(standardError);
    }

    return standardError;
  }
}
```

## Integration Points

### API Error Middleware

```javascript
// src/api/middleware/errorHandler.js

const errorHandler = (err, req, res, next) => {
  const error = ErrorHandler.formatForInterface(err, 'rest');
  const statusCode = determineHttpStatus(err);
  
  res.status(statusCode).json(error);
};

export default errorHandler;
```

### WebSocket Error Handling

```javascript
// src/api/websocket/server.js

ws.on('error', (err) => {
  const error = ErrorHandler.formatForInterface(err, 'websocket');
  ws.send(JSON.stringify(error));
});
```

### Error Storage Schema

```javascript
// src/models/Error.js

const ErrorSchema = new Schema({
  timestamp: { 
    type: Number,
    required: true,
    index: true
  },
  code: {
    type: String,
    required: true,
    index: true
  },
  message: {
    type: String,
    required: true
  },
  details: Schema.Types.Mixed,
  context: Schema.Types.Mixed
});

// Indexes for efficient querying
ErrorSchema.index({ timestamp: 1, code: 1 });
ErrorSchema.index({ 'context.callId': 1 });
```

## Error Categories and Codes

### System Errors
- `SYSTEM_UNAVAILABLE`: System-wide unavailability
- `RESOURCE_EXHAUSTED`: System resource limits reached
- `INTERNAL_ERROR`: Unhandled internal error

### Authentication Errors
- `AUTH_REQUIRED`: Authentication required
- `AUTH_INVALID`: Invalid authentication credentials
- `AUTH_EXPIRED`: Authentication token expired

### Data Errors
- `INVALID_REQUEST`: Malformed request
- `VALIDATION_FAILED`: Request validation failed
- `NOT_FOUND`: Requested resource not found
- `CONFLICT`: Resource conflict

### Audio Processing Errors
- `AUDIO_PROCESSING_FAILED`: Audio processing failure
- `AUDIO_STORAGE_FAILED`: Audio storage failure
- `AUDIO_FORMAT_INVALID`: Invalid audio format
- `AUDIO_CORRUPTED`: Audio file corruption detected

### Transcription Errors
- `TRANSCRIPTION_FAILED`: Transcription processing failure
- `TRANSCRIPTION_TIMEOUT`: Transcription time limit exceeded
- `TRANSCRIPTION_UNSUPPORTED`: Unsupported audio for transcription

## Error Handling Best Practices

### General Guidelines

1. Always use the ErrorHandler service for error processing
2. Include relevant context with every error
3. Use appropriate error codes from defined categories
4. Ensure user messages are clear and actionable
5. Log all errors for monitoring and analysis

### Development vs Production

Development environments include additional information:
- Full stack traces
- Detailed error context
- Database query information
- Performance metrics

Production environments limit sensitive information:
- Generic user-facing messages
- Limited technical details
- Sanitized error context
- Error reference codes for support

### Error Recovery

1. Implement automatic retry logic where appropriate
2. Maintain system state consistency during errors
3. Provide clear feedback for user-correctable errors
4. Log recovery attempts and outcomes

### Error Monitoring

1. Track error frequencies and patterns
2. Monitor error resolution times
3. Analyze error impact on system performance
4. Generate error trend reports

## Implementation Timeline

Phase 1:
- Implement ErrorHandler service
- Update API middleware
- Add basic error logging

Phase 2:
- Implement error storage
- Add WebSocket error handling
- Create error monitoring dashboard

Phase 3:
- Implement advanced error analysis
- Add error recovery mechanisms
- Create error reporting system

## Maintenance and Evolution

1. Regular review of error patterns
2. Updates to error categories and codes
3. Refinement of error messages
4. Enhancement of error analysis capabilities

This specification provides a foundation for consistent, informative error handling across the TR-ENGINE system while maintaining flexibility for future enhancements and specific use cases.

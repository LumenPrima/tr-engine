const WebSocket = require('ws');
const logger = require('../../utils/logger');
const EventHandlers = require('../../services/events/handlers');
const activeCallManager = require('../../services/state/ActiveCallManager');
const systemManager = require('../../services/state/SystemManager');
const unitManager = require('../../services/state/UnitManager');

class WebSocketServer {
    constructor(server) {
        this.wss = new WebSocket.Server({ server });
        this.eventHandlers = new EventHandlers(this.wss);
        this.setupWebSocketServer();
    }

    close() {
        return new Promise((resolve) => {
            this.wss.close(() => {
                logger.info('WebSocket server closed');
                resolve();
            });
        });
    }

    setupWebSocketServer() {
        this.wss.on('connection', this.handleConnection.bind(this));
        this.wss.on('error', this.handleServerError.bind(this));
    }

    handleConnection(ws, req) {
        const clientIp = req.socket.remoteAddress;
        logger.info(`New WebSocket connection from ${clientIp}`);

        // Send initial state to the new client
        this.sendInitialState(ws);

        // Setup client event handlers
        ws.on('message', (message) => this.handleClientMessage(ws, message));
        ws.on('close', () => this.handleClientDisconnect(ws));
        ws.on('error', (error) => this.handleClientError(ws, error));
    }

    async sendInitialState(ws) {
        try {
            // Get current state from managers
            const activeCalls = await activeCallManager.getActiveCalls();
            const activeSystems = await systemManager.getActiveSystems();
            const activeUnits = await unitManager.getActiveUnits();

            // Send initial state messages
            if (ws.readyState === WebSocket.OPEN) {
                ws.send(JSON.stringify({
                    type: 'initial.state',
                    timestamp: new Date().toISOString(),
                    data: {
                        calls: activeCalls,
                        systems: activeSystems,
                        units: activeUnits
                    }
                }));
                logger.debug('Sent initial state to new client');
            }
        } catch (err) {
            logger.error('Error sending initial state:', err);
        }
    }

    handleClientMessage(ws, message) {
        try {
            const data = JSON.parse(message);
            logger.debug('Received WebSocket message:', data);

            // Handle client requests
            switch (data.type) {
                case 'get.active_calls':
                    this.handleGetActiveCalls(ws);
                    break;
                case 'get.system_status':
                    this.handleGetSystemStatus(ws);
                    break;
                case 'get.unit_status':
                    this.handleGetUnitStatus(ws, data.unit);
                    break;
                default:
                    logger.warn(`Unknown message type: ${data.type}`);
            }
        } catch (err) {
            logger.error('Error handling client message:', err);
            this.sendError(ws, 'Invalid message format');
        }
    }

    handleClientDisconnect(ws) {
        logger.info('Client disconnected from WebSocket');
    }

    handleClientError(ws, error) {
        logger.error('WebSocket client error:', error);
    }

    handleServerError(error) {
        logger.error('WebSocket server error:', error);
    }

    // Client request handlers
    async handleGetActiveCalls(ws) {
        try {
            const calls = await activeCallManager.getActiveCalls();
            this.sendResponse(ws, 'active_calls', calls);
        } catch (err) {
            logger.error('Error getting active calls:', err);
            this.sendError(ws, 'Failed to get active calls');
        }
    }

    async handleGetSystemStatus(ws) {
        try {
            const systems = await systemManager.getActiveSystems();
            this.sendResponse(ws, 'system_status', systems);
        } catch (err) {
            logger.error('Error getting system status:', err);
            this.sendError(ws, 'Failed to get system status');
        }
    }

    async handleGetUnitStatus(ws, unit) {
        try {
            const unitState = await unitManager.getUnitState(unit);
            this.sendResponse(ws, 'unit_status', unitState);
        } catch (err) {
            logger.error('Error getting unit status:', err);
            this.sendError(ws, 'Failed to get unit status');
        }
    }

    // Helper methods for sending responses
    sendResponse(ws, type, data) {
        if (ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({
                type,
                timestamp: new Date().toISOString(),
                data
            }));
        }
    }

    sendError(ws, message) {
        if (ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({
                type: 'error',
                timestamp: new Date().toISOString(),
                error: message
            }));
        }
    }

    // Method to broadcast to all clients
    broadcast(type, data) {
        this.wss.clients.forEach(client => {
            if (client.readyState === WebSocket.OPEN) {
                client.send(JSON.stringify({
                    type,
                    timestamp: new Date().toISOString(),
                    data
                }));
            }
        });
    }
}

module.exports = WebSocketServer;

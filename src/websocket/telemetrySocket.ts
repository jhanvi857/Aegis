// WebSocket Client for Real-time Telemetry Data Streams
type MetricHandler = (data: any) => void;

class TelemetrySocketManager {
  private socket: WebSocket | null = null;
  private listeners: Set<MetricHandler> = new Set();
  private reconnectInterval: number = 3000;
  private url: string = import.meta.env.VITE_WS_URL || 'ws://localhost:8080/ws';

  public connect() {
    try {
      this.socket = new WebSocket(this.url);

      this.socket.onopen = () => {
        console.log('[Telemetry WS] Connected to backend telemetry socket');
      };

      this.socket.onmessage = (event) => {
        try {
          const parsed = JSON.parse(event.data);
          this.listeners.forEach((listener) => listener(parsed));
        } catch (e) {
          console.error('[Telemetry WS] Failed to parse message:', e);
        }
      };

      this.socket.onclose = () => {
        console.log('[Telemetry WS] Disconnected. Reconnecting in 3s...');
        setTimeout(() => this.connect(), this.reconnectInterval);
      };

      this.socket.onerror = (err) => {
        console.warn('[Telemetry WS] Connection error (falling back to simulation mode):', err);
      };
    } catch (e) {
      console.warn('[Telemetry WS] Native WebSocket initialization skipped:', e);
    }
  }

  public subscribe(handler: MetricHandler) {
    this.listeners.add(handler);
    return () => {
      this.listeners.delete(handler);
    };
  }

  public disconnect() {
    if (this.socket) {
      this.socket.close();
      this.socket = null;
    }
  }
}

export const telemetrySocket = new TelemetrySocketManager();

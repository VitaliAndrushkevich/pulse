export type ConnectionStatus = 'connecting' | 'connected' | 'disconnected' | 'auth_expired';

let status = $state<ConnectionStatus>('disconnected');
let lastConnected = $state<Date | null>(null);

function setStatus(newStatus: ConnectionStatus): void {
	status = newStatus;
	if (newStatus === 'connected') {
		lastConnected = new Date();
	}
}

export const connectionStore = {
	get status(): ConnectionStatus {
		return status;
	},
	get lastConnected(): Date | null {
		return lastConnected;
	},
	setStatus
};

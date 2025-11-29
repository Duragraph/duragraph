// API Client for DuraGraph Backend

import type {
	Assistant,
	CreateAssistantRequest,
	UpdateAssistantRequest,
	Thread,
	CreateThreadRequest,
	UpdateThreadRequest,
	AddMessageRequest,
	Run,
	CreateRunRequest,
	SubmitToolOutputsRequest,
	Graph,
	SaveGraphRequest,
	HealthResponse
} from './types';

const API_BASE = import.meta.env.VITE_API_URL || 'http://localhost:8081';

export class ApiClient {
	private baseUrl: string;
	private authToken: string | null = null;

	constructor(baseUrl: string = API_BASE) {
		this.baseUrl = baseUrl;
		// Try to load token from localStorage
		if (typeof window !== 'undefined') {
			this.authToken = localStorage.getItem('auth_token');
		}
	}

	setAuthToken(token: string | null) {
		this.authToken = token;
		if (typeof window !== 'undefined') {
			if (token) {
				localStorage.setItem('auth_token', token);
			} else {
				localStorage.removeItem('auth_token');
			}
		}
	}

	getAuthToken(): string | null {
		return this.authToken;
	}

	private async request<T>(path: string, options?: RequestInit): Promise<T> {
		const headers: Record<string, string> = {
			'Content-Type': 'application/json',
			...(options?.headers as Record<string, string>)
		};

		// Add authorization header if token exists
		if (this.authToken) {
			headers['Authorization'] = `Bearer ${this.authToken}`;
		}

		const response = await fetch(`${this.baseUrl}${path}`, {
			...options,
			headers
		});

		if (response.status === 401) {
			// Unauthorized - clear token and throw error
			this.setAuthToken(null);
			throw new Error('Unauthorized - please login again');
		}

		if (!response.ok) {
			const error = await response.json().catch(() => ({
				error: 'unknown_error',
				message: response.statusText
			}));
			throw new Error(error.message || `API Error: ${response.statusText}`);
		}

		// Handle empty responses (204 No Content)
		if (response.status === 204) {
			return {} as T;
		}

		return response.json();
	}

	// ========================================================================
	// Health Check
	// ========================================================================

	async health(): Promise<HealthResponse> {
		return this.request<HealthResponse>('/health');
	}

	// ========================================================================
	// Assistants
	// ========================================================================

	async getAssistants(): Promise<Assistant[]> {
		return this.request<Assistant[]>('/api/v1/assistants');
	}

	async getAssistant(id: string): Promise<Assistant> {
		return this.request<Assistant>(`/api/v1/assistants/${id}`);
	}

	async createAssistant(data: CreateAssistantRequest): Promise<Assistant> {
		return this.request<Assistant>('/api/v1/assistants', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	}

	async updateAssistant(id: string, data: UpdateAssistantRequest): Promise<Assistant> {
		return this.request<Assistant>(`/api/v1/assistants/${id}`, {
			method: 'PATCH',
			body: JSON.stringify(data)
		});
	}

	async deleteAssistant(id: string): Promise<void> {
		return this.request<void>(`/api/v1/assistants/${id}`, {
			method: 'DELETE'
		});
	}

	// ========================================================================
	// Threads
	// ========================================================================

	async getThreads(): Promise<Thread[]> {
		return this.request<Thread[]>('/api/v1/threads');
	}

	async getThread(id: string): Promise<Thread> {
		return this.request<Thread>(`/api/v1/threads/${id}`);
	}

	async createThread(data?: CreateThreadRequest): Promise<Thread> {
		return this.request<Thread>('/api/v1/threads', {
			method: 'POST',
			body: JSON.stringify(data || {})
		});
	}

	async updateThread(id: string, data: UpdateThreadRequest): Promise<Thread> {
		return this.request<Thread>(`/api/v1/threads/${id}`, {
			method: 'PATCH',
			body: JSON.stringify(data)
		});
	}

	async addMessage(threadId: string, data: AddMessageRequest): Promise<Thread> {
		return this.request<Thread>(`/api/v1/threads/${threadId}/messages`, {
			method: 'POST',
			body: JSON.stringify(data)
		});
	}

	async deleteThread(id: string): Promise<void> {
		return this.request<void>(`/api/v1/threads/${id}`, {
			method: 'DELETE'
		});
	}

	// Messages
	async getMessages(threadId: string): Promise<any[]> {
		// TODO: Update with proper Message type when backend implements this
		return this.request<any[]>(`/api/v1/threads/${threadId}/messages`);
	}

	async createMessage(threadId: string, data: AddMessageRequest): Promise<any> {
		return this.request<any>(`/api/v1/threads/${threadId}/messages`, {
			method: 'POST',
			body: JSON.stringify(data)
		});
	}

	// ========================================================================
	// Runs
	// ========================================================================

	async getRuns(threadId?: string): Promise<Run[]> {
		const path = threadId ? `/api/v1/threads/${threadId}/runs` : '/api/v1/runs';
		return this.request<Run[]>(path);
	}

	async getRun(id: string): Promise<Run> {
		return this.request<Run>(`/api/v1/runs/${id}`);
	}

	async createRun(data: CreateRunRequest): Promise<Run> {
		return this.request<Run>('/api/v1/runs', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	}

	async submitToolOutputs(runId: string, data: SubmitToolOutputsRequest): Promise<{ status: string }> {
		return this.request<{ status: string }>(`/api/v1/runs/${runId}/submit_tool_outputs`, {
			method: 'POST',
			body: JSON.stringify(data)
		});
	}

	async cancelRun(runId: string): Promise<Run> {
		return this.request<Run>(`/api/v1/runs/${runId}/cancel`, {
			method: 'POST'
		});
	}

	// ========================================================================
	// Streaming
	// ========================================================================

	streamRun(runId: string): EventSource {
		const url = `${this.baseUrl}/api/v1/stream?run_id=${runId}`;
		return new EventSource(url);
	}

	// ========================================================================
	// Graphs (if backend supports these endpoints)
	// ========================================================================

	async getGraphs(): Promise<Graph[]> {
		try {
			return this.request<Graph[]>('/api/v1/graphs');
		} catch (error) {
			// If endpoint doesn't exist yet, return empty array
			console.warn('Graph endpoints not implemented yet:', error);
			return [];
		}
	}

	async getGraph(id: string): Promise<Graph> {
		return this.request<Graph>(`/api/v1/graphs/${id}`);
	}

	async saveGraph(data: SaveGraphRequest): Promise<Graph> {
		return this.request<Graph>('/api/v1/graphs', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	}

	async updateGraph(id: string, data: Partial<SaveGraphRequest>): Promise<Graph> {
		return this.request<Graph>(`/api/v1/graphs/${id}`, {
			method: 'PATCH',
			body: JSON.stringify(data)
		});
	}

	async deleteGraph(id: string): Promise<void> {
		return this.request<void>(`/api/v1/graphs/${id}`, {
			method: 'DELETE'
		});
	}
}

// Export singleton instance
export const api = new ApiClient();

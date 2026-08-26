export interface ClientOptions {
  baseUrl: string;
  token?: string;
}

export interface AuthData {
  email: string;
  password: string;
  username?: string;
}

export interface RemittanceData {
  recipient_address: string;
  amount: number;
  currency?: string;
  deadline?: string;
}

export interface InvoiceData {
  amount: number;
  currency?: string;
  due_date?: string;
  description?: string;
}

export interface WebhookData {
  url: string;
  events: string[];
  description?: string;
}

export interface QueryParams {
  [key: string]: string | number | undefined;
}

export class GpayRemitClient {
  constructor(opts: ClientOptions);
  setToken(token: string): void;

  auth: {
    register(data: AuthData): Promise<any>;
    login(data: AuthData): Promise<any>;
    refresh(data: { refresh_token?: string }): Promise<any>;
    logout(): Promise<any>;
  };

  remittances: {
    create(data: RemittanceData): Promise<any>;
    send(data: RemittanceData): Promise<any>;
    get(id: string): Promise<any>;
    list(params?: QueryParams): Promise<any>;
    complete(id: string): Promise<any>;
  };

  invoices: {
    create(data: InvoiceData): Promise<any>;
    list(params?: QueryParams): Promise<any>;
    get(id: string): Promise<any>;
  };

  fees: {
    calculate(params: QueryParams): Promise<any>;
    exchangeRate(params: QueryParams): Promise<any>;
  };

  paymentRequests: {
    create(data: any): Promise<any>;
    list(): Promise<any>;
    get(id: string): Promise<any>;
    accept(id: string): Promise<any>;
    reject(id: string): Promise<any>;
    cancel(id: string): Promise<any>;
  };

  webhooks: {
    create(data: WebhookData): Promise<any>;
    list(): Promise<any>;
    get(id: string): Promise<any>;
    update(id: string, data: Partial<WebhookData>): Promise<any>;
    delete(id: string): Promise<any>;
    deliveries(id: string): Promise<any>;
    retryDelivery(deliveryId: string): Promise<any>;
  };

  analytics: {
    volume(params?: QueryParams): Promise<any>;
    fees(params?: QueryParams): Promise<any>;
    successRate(): Promise<any>;
    topCorridors(): Promise<any>;
  };

  audit: {
    list(params?: QueryParams): Promise<any>;
  };

  health: {
    check(): Promise<any>;
    ready(): Promise<any>;
    live(): Promise<any>;
  };
}

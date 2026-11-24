// Base Ledger service structure
export class LedgerWalletService {
  private transport: any | null = null;
  private str: any | null = null;

  async connect(): Promise<{ publicKey: string }> {
    try {
      if (!this.transport) {
        const [{ default: TransportWebUSB }, { default: Str }] = await Promise.all([
          import("@ledgerhq/hw-transport-webusb"),
          import("@ledgerhq/hw-app-str"),
        ]);
        this.transport = await TransportWebUSB.create();
        this.str = new Str(this.transport);
      }
      
      const result = await this.str.getPublicKey("44'/148'/0'");
      return { publicKey: result.publicKey };
    } catch (error: any) {
      console.error("Ledger connection failed:", error);
      throw new Error(`Failed to connect to Ledger: ${error.message || 'Unknown error'}`);
    }
  }

  async disconnect(): Promise<void> {
    if (this.transport) {
      await this.transport.close();
      this.transport = null;
      this.str = null;
    }
  }

  async signTransaction(transactionXdr: string): Promise<string> {
    if (!this.str) {
      throw new Error("Ledger is not connected");
    }

    try {
      // The transaction is passed as base64 XDR string.
      // Ledger expects a signature for the transaction buffer.
      const result = await this.str.signTransaction("44'/148'/0'", transactionXdr);
      
      if (result && result.signature) {
        return result.signature.toString("base64");
      }
      throw new Error("Failed to get a valid signature from Ledger");
    } catch (error: any) {
      console.error("Ledger signing failed:", error);
      throw new Error(`Failed to sign transaction: ${error.message || 'Unknown error'}`);
    }
  }
}

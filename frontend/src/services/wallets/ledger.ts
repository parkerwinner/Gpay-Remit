// Base Ledger service structure
import TransportWebUSB from "@ledgerhq/hw-transport-webusb";
import Str from "@ledgerhq/hw-app-stellar";

export class LedgerWalletService {
  private transport: any | null = null;
  private str: any | null = null;

  constructor() {}

  async connect(): Promise<{ publicKey: string }> {
    try {
      if (!this.transport) {
        this.transport = await TransportWebUSB.create();
      }
      this.str = new Str(this.transport);
      
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

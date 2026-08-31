// Base Ledger service structure
import TransportWebUSB from "@ledgerhq/hw-transport-webusb";
import Str from "@ledgerhq/hw-app-stellar";

export class LedgerWalletService {
  private transport: any | null = null;
  private str: any | null = null;

  constructor() {}
}

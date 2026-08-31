import React, { createContext, useContext, useState, useMemo } from 'react';
import { LedgerWalletService } from '../services/wallets/ledger';

const WalletContext = createContext();

export const useWallet = () => {
  const context = useContext(WalletContext);
  if (!context) {
    throw new Error('useWallet must be used within a WalletProvider');
  }
  return context;
};

export const WalletProvider = ({ children }) => {
  const [publicKey, setPublicKey] = useState(null);
  const [isConnecting, setIsConnecting] = useState(false);
  const [error, setError] = useState(null);

  const ledgerService = useMemo(() => new LedgerWalletService(), []);

  const connectLedger = async () => {
    setIsConnecting(true);
    setError(null);
    try {
      const result = await ledgerService.connect();
      setPublicKey(result.publicKey);
    } catch (err) {
      setError(err.message || 'Failed to connect to Ledger');
    } finally {
      setIsConnecting(false);
    }
  };

  const disconnect = async () => {
    await ledgerService.disconnect();
    setPublicKey(null);
  };

  const signTransaction = async (txXdr) => {
    return await ledgerService.signTransaction(txXdr);
  };

  const value = {
    publicKey,
    isConnecting,
    error,
    connectLedger,
    disconnect,
    signTransaction,
  };

  return (
    <WalletContext.Provider value={value}>
      {children}
    </WalletContext.Provider>
  );
};

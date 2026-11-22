import React from "react";
import { render, screen, act } from "@testing-library/react";
import { WalletProvider, useWallet } from "./WalletContext";

jest.mock("../services/wallets/ledger.ts", () => ({
  LedgerWalletService: jest.fn().mockImplementation(() => ({
    connect: jest.fn().mockResolvedValue({ publicKey: "GLEDGERTESTADDRESS12345" }),
    disconnect: jest.fn().mockResolvedValue(true),
    signTransaction: jest.fn().mockResolvedValue("signed-xdr-ledger"),
  })),
}));

jest.mock("../services/wallets/freighter.ts", () => ({
  FreighterWalletService: jest.fn().mockImplementation(() => ({
    connect: jest.fn().mockResolvedValue({ publicKey: "GFREIGHTERTESTADDRESS12345" }),
    disconnect: jest.fn().mockResolvedValue(true),
    signTransaction: jest.fn().mockResolvedValue("signed-xdr-freighter"),
  })),
}));

jest.mock("../services/wallets/albedo.ts", () => ({
  AlbedoWalletService: jest.fn().mockImplementation(() => ({
    connect: jest.fn().mockResolvedValue({ publicKey: "GALBEDOTESTADDRESS12345" }),
    disconnect: jest.fn().mockResolvedValue(true),
    signTransaction: jest.fn().mockResolvedValue("signed-xdr-albedo"),
  })),
}));

jest.mock("../services/wallets/xbull.ts", () => ({
  XBullWalletService: jest.fn().mockImplementation(() => ({
    connect: jest.fn().mockResolvedValue({ publicKey: "GXBULLTESTADDRESS12345" }),
    disconnect: jest.fn().mockResolvedValue(true),
    signTransaction: jest.fn().mockResolvedValue("signed-xdr-xbull"),
  })),
}));

function ConsumerComponent() {
  const { publicKey, walletType, isConnected, connect, disconnect } = useWallet();

  return (
    <div>
      <div data-testid="status">{isConnected ? "Connected" : "Disconnected"}</div>
      <div data-testid="public-key">{publicKey || "none"}</div>
      <div data-testid="wallet-type">{walletType || "none"}</div>
      <button onClick={() => connect("freighter")}>Connect Freighter</button>
      <button onClick={() => disconnect()}>Disconnect</button>
    </div>
  );
}

describe("WalletContext", () => {
  test("throws error when useWallet is used outside WalletProvider", () => {
    const consoleError = jest.spyOn(console, "error").mockImplementation(() => {});

    expect(() => render(<ConsumerComponent />)).toThrow(
      "useWallet must be used within a WalletProvider"
    );

    consoleError.mockRestore();
  });

  test("connects and disconnects wallet successfully", async () => {
    render(
      <WalletProvider>
        <ConsumerComponent />
      </WalletProvider>
    );

    expect(screen.getByTestId("status")).toHaveTextContent("Disconnected");
    expect(screen.getByTestId("public-key")).toHaveTextContent("none");

    await act(async () => {
      screen.getByText("Connect Freighter").click();
    });

    expect(screen.getByTestId("status")).toHaveTextContent("Connected");
    expect(screen.getByTestId("public-key")).toHaveTextContent("GFREIGHTERTESTADDRESS12345");
    expect(screen.getByTestId("wallet-type")).toHaveTextContent("freighter");

    await act(async () => {
      screen.getByText("Disconnect").click();
    });

    expect(screen.getByTestId("status")).toHaveTextContent("Disconnected");
    expect(screen.getByTestId("public-key")).toHaveTextContent("none");
  });
});

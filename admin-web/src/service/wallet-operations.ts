export interface WalletAccount {
  playerId: number;
  name: string;
  balanceCents: number;
  revision: number;
}
export interface WalletLedgerItem {
  id: string | number;
  kind: 'credit' | 'payment';
  amountCents: number;
  balanceCents: number;
  createdAt: number;
  label: string;
  reasonId?: string;
  orderId?: string;
}
export interface WalletLedger {
  balanceCents: number;
  summary: {
    startAt: number;
    endAt: number;
    incomeCents: number;
    expenseCents: number;
    netCents: number;
    transactionCount: number;
  };
  items: WalletLedgerItem[];
  total: number;
}
export interface WalletReason {
  id: string;
  label: string;
  minCents: number;
  maxCents: number;
  weight: number;
  enabled: boolean;
  version: string | number;
}
export function money(cents: number | undefined) {
  return (Number(cents || 0) / 100).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}

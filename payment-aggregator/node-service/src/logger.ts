/**
 * Payment Logger - Processes payments one at a time
 */

export interface Payment {
  id: string;
  provider: string;
  amount: number;
  currency: string;
  status: string;
  customerEmail: string;
  createdAt: string;
  processedAt: string;
  metadata: Record<string, string>;
}

export interface PaymentEvent {
  payment: Payment;
  eventType: string;
  topic: string;
}

// Provider enum mapping
const PROVIDERS: Record<number, string> = {
  0: 'UNKNOWN',
  1: 'STRIPE',
  2: 'RAZORPAY',
  3: 'PAYPAL',
};

// Status enum mapping
const STATUSES: Record<number, string> = {
  0: 'UNKNOWN',
  1: 'PENDING',
  2: 'COMPLETED',
  3: 'FAILED',
  4: 'REFUNDED',
};

// Format currency
function formatAmount(amount: number, currency: string): string {
  const symbols: Record<string, string> = {
    USD: '$',
    EUR: '€',
    GBP: '£',
    INR: '₹',
  };
  const symbol = symbols[currency] || currency + ' ';
  return `${symbol}${amount.toFixed(2)}`;
}

// Process a single payment
export function processPayment(event: PaymentEvent): void {
  const { payment, eventType } = event;
  
  const provider = PROVIDERS[payment.provider as unknown as number] || payment.provider;
  const status = STATUSES[payment.status as unknown as number] || payment.status;
  const amount = formatAmount(payment.amount, payment.currency);
  
  const timestamp = new Date().toISOString();
  const tag = eventType === 'new' ? '🆕 NEW' : '📦 EXISTING';
  
  console.log(`
┌─────────────────────────────────────────────────────────────┐
│ ${tag} PAYMENT                                              
├─────────────────────────────────────────────────────────────┤
│ ID:        ${payment.id}
│ Provider:  ${provider}
│ Amount:    ${amount} ${payment.currency}
│ Status:    ${status}
│ Customer:  ${payment.customerEmail}
│ Time:      ${timestamp}
└─────────────────────────────────────────────────────────────┘`);

  // Here you could add additional processing:
  // - Save to database
  // - Send notifications
  // - Update analytics
  // - Forward to another service
}

// Simple one-line log format
export function logPaymentSimple(event: PaymentEvent): void {
  const { payment, eventType, topic } = event;
  const provider = PROVIDERS[payment.provider as unknown as number] || payment.provider;
  const status = STATUSES[payment.status as unknown as number] || payment.status;

  const icon = eventType === 'new' ? '🆕' : '📦';

  console.log(
    `${icon} [${topic}] ${payment.id} | ${provider} | ${formatAmount(payment.amount, payment.currency)} ${payment.currency} | ${status}`
  );
}
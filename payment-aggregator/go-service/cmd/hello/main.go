package main

import (
    "fmt"
    "time"
)

// Payment represents a normalized payment
type Payment struct {
    ID        string
    Provider  string
    Amount    float64
    Currency  string
    Status    string
    CreatedAt time.Time
}

// Display returns a formatted string
func (p Payment) Display() string {
    return fmt.Sprintf("[%s] %s: %.2f %s - %s",
        p.Provider, p.ID, p.Amount, p.Currency, p.Status)
}

// Normalize converts cents to dollars
func (p *Payment) Normalize() {
    if p.Amount > 100 { // Assuming cents if > 100
        p.Amount = p.Amount / 100
    }
    // Uppercase currency
    p.Currency = fmt.Sprintf("%s", p.Currency)
}

func main() {
    // Create a payment (simulating raw Kafka data)
    payment := Payment{
        ID:        "pay_abc123",
        Provider:  "stripe",
        Amount:    5000, // cents
        Currency:  "usd",
        Status:    "succeeded",
        CreatedAt: time.Now(),
    }

    fmt.Println("Before normalization:")
    fmt.Println(payment.Display())

    // Normalize it
    payment.Normalize()
    payment.Status = "COMPLETED"
    payment.Provider = "STRIPE"

    fmt.Println("\nAfter normalization:")
    fmt.Println(payment.Display())
}
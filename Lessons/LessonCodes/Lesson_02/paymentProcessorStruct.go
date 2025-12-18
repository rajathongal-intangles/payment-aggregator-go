 package main

  import "fmt"

  // 1. Define the struct we'll process
  type Payment struct {
      ID       string
      Amount   float64
      Currency string
  }

  // 2. Define the interface (the contract)
  type PaymentProcessor interface {
      Process(payment Payment) error
      Refund(paymentID string) error
  }

  // 3. Stripe implements the interface
  type StripeProcessor struct{}

  func (s StripeProcessor) Process(p Payment) error {
      fmt.Printf("💳 STRIPE: Processing %s for %.2f %s\n", p.ID, p.Amount, p.Currency)
      return nil
  }

  func (s StripeProcessor) Refund(id string) error {
      fmt.Printf("💳 STRIPE: Refunding %s\n", id)
      return nil
  }

  // 4. Razorpay ALSO implements the interface
  type RazorpayProcessor struct{}

  func (r RazorpayProcessor) Process(p Payment) error {
      fmt.Printf("🏦 RAZORPAY: Processing %s for %.2f %s\n", p.ID, p.Amount, p.Currency)
      return nil
  }

  func (r RazorpayProcessor) Refund(id string) error {
      fmt.Printf("🏦 RAZORPAY: Refunding %s\n", id)
      return nil
  }

  // 5. THIS IS THE MAGIC - one function works with ANY processor!
  func handlePayment(processor PaymentProcessor, p Payment) {
      err := processor.Process(p)
      if err != nil {
          fmt.Println("Error:", err)
      }
  }

  func main() {
      // Create a payment
      payment := Payment{
          ID:       "pay_123",
          Amount:   500.00,
          Currency: "INR",
      }

      // Use Stripe
      stripe := StripeProcessor{}
      handlePayment(stripe, payment)

      // Use Razorpay - SAME function, different processor!
      razorpay := RazorpayProcessor{}
      handlePayment(razorpay, payment)

      // You can also store any processor in a variable of interface type
      var processor PaymentProcessor

      processor = stripe
      processor.Process(payment)  // Uses Stripe

      processor = razorpay
      processor.Process(payment)  // Uses Razorpay
  }
package main

import (
	"fmt"
 	"errors"
	"time"
)

// 2.  Structs and classes ( like classess, but no inheritence )
	type Payment struct {
		ID string
		Amount float64
		Currency string
		Status string
	}

// 3. Methods (functions attached to structs)

	func (p Payment) Display() string {
		return fmt.Sprintf("%s: %.2f %s", p.ID, p.Amount, p.Currency)
	}

	func (p *Payment) MarkCompleted() { // pointer reciever can modify the struct 
		p.Status = "Completed"
	}

	// 4. Interfaces
	type PaymentProcessor interface { // Interface = Contract
		Process(payment Payment) error 
		Refund (paymentID string) error
	}

	type StripeProcessor struct {} // Any struct implementing these methods satisfies the interface


	func (s StripeProcessor) Process(p Payment) error {
	fmt.Println("Processing via Stripe:", p.ID)
	return nil
	}

	func (s StripeProcessor) Refund(id string) error {
	fmt.Println("Refunding via Stripe:", id)
	return nil
	}

	// Now StripeProcessor "implements" PaymentProcessor
	// No explicit "implements" keyword needed!
	var processor PaymentProcessor = StripeProcessor{}

	func divide(a, b float64) (float64, error) {
		if(b == 0) {
			return 0, errors.New("Division by Zero")
		}

		return a / b, nil
	}

																																									
	// Simulate payment processing (takes 2 seconds)                                                                                                                  
	func processPayment(p Payment) {                                                                                                                                  
		fmt.Printf("⏳ Processing %s...\n", p.ID)                                                                                                                     
		time.Sleep(2 * time.Second)  // Simulate API call delay                                                                                                       
		fmt.Printf("✅ Done processing %s\n", p.ID)                                                                                                                   
	} 


func main() {
	fmt.Println("Hello World !")

	// 1. Variables and Types

	var name string = "payment"
	nameImplicit := "payment_implicit_datatype_assignment"

	fmt.Println(name, nameImplicit)

	var amount float64 = 99.9999
	var count int = 42

	amountImplicit:= 99
	countImplicit:= 11

	fmt.Println(amount, amountImplicit, count, countImplicit)

	// constant vars
	const maxRetries = 9

	fmt.Println(maxRetries)

	p1:= Payment{ // Create Instances
		ID: "pay_321",
		Amount: 50.00,
		Currency: "USD",
		Status: "Completed",
	}

	p2:= Payment{"12_ppo", 69.99, "INR", "NotCompleted"}
	p3 := Payment{ID: "pay_003", Amount: 300, Currency: "EUR", Status: "Pending"} 

	fmt.Println(p1.ID, p2.ID)

	fmt.Println(p1.Display())

	resultOne, errOne := divide(10, 0)
	if errOne != nil {
		fmt.Println("Error: ", errOne)
	}
	fmt.Println("Result: ", resultOne)

	resultTwo, errTwo := divide(10, 2)
	if errTwo != nil {
		fmt.Println("Error: ", errTwo)
	}
	fmt.Println("Result: ", resultTwo)


	// Go Routines 
	// ==========================================                                                                                                                 
	// BLOCKING (Sequential) - Takes 6 seconds                                                                                                                    
	// ==========================================                                                                                                                 
	fmt.Println("\n--- BLOCKING (one by one) ---")                                                                                                                
	start := time.Now()                                                                                                                                           
																																								
	processPayment(p1)  // Wait 2 sec                                                                                                                             
	processPayment(p2)  // Wait 2 sec                                                                                                                             
	processPayment(p3)  // Wait 2 sec                                                                                                                             
																																								
	fmt.Printf("Total time: %v\n", time.Since(start))  // ~6 seconds    
	
	// ==========================================                                                                                                                 
	// CONCURRENT (Goroutines) - Takes 2 seconds                                                                                                                  
	// ==========================================                                                                                                                 
	fmt.Println("\n--- CONCURRENT (all at once) ---")                                                                                                             
	start = time.Now()                                                                                                                                            
																																								
	go processPayment(p1)  // Starts immediately                                                                                                                  
	go processPayment(p2)  // Starts immediately                                                                                                                  
	go processPayment(p3)  // Starts immediately                                                                                                                  
																																								
	// IMPORTANT: Wait for goroutines to finish!                                                                                                                  
	time.Sleep(3 * time.Second)                                                                                                                                   
																																								
	fmt.Printf("Total time: %v\n", time.Since(start))  // ~2 seconds   
}
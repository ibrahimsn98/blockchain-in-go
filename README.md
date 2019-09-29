## Usage
Get the balance for an address
```
$ go run main.go getbalance -address ADDRESS
```

Create a blockchain and send genesis reward to address
```
$ go run main.go createblockchain -address ADDRESS
```

Print the blocks in the chain
```
$ go run main.go printchain
```

Send amount of coins
```
$ go run main.go send -from FROM -to TO -amount AMOUNT
```

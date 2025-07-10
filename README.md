# go-micro Services
This is a simple project I tried while learning about go-micro and the micro toolkit. The framework project a simple store packet with great plugins but
I really wanted to try something like combining interesting project. So I opted for entgo to take care of the database interaction.

## First clone this repo
## Install the micro toolkit given that you already have go installed
```
go install github.com/micro/micro/v5@latest
```
## Then you move the the root of the clone project
```
cd services
```
## finally you run
```
micro run
```


## Go to the  micro repo for more information: https://github.com/micro/micro


Personally I think what is really missing for a project like go-micro is the entry level required. If given enough documentation were provided, I think it will get much more adoption

Also this project is not finished yet, I wanted to build all services using grpc and then based on the requirements of the web/mobile app, I could build an api gateway (inspired from https://github.com/micro/blog) where the different rpc client will be called and exposed needed services.

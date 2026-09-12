# 🌐 Multi-Language Client Guide

BeastDB is architected to be completely **language-agnostic**. The server engine runs as an independent daemon or container, exposing its database operations via high-performance **gRPC over HTTP/2** defined by [api/proto/beastdb.proto](../api/proto/beastdb.proto).

You can connect to BeastDB from **Python, Node.js / TypeScript, Rust, Java, C#, Go**, or any language with gRPC support.

---

## 1. Protobuf Contract

The service definition in pi/proto/beastdb.proto exposes four core database RPCs:

`protobuf
service BeastDBService {
  rpc Get (GetRequest) returns (GetResponse);
  rpc Put (PutRequest) returns (PutResponse);
  rpc Delete (DeleteRequest) returns (DeleteResponse);
  rpc Scan (ScanRequest) returns (stream KeyValueResponse);
}
`

---

## 2. Python Client

### Setup
`ash
pip install grpcio grpcio-tools protobuf
`

### Generate Stubs
`ash
python -m grpc_tools.protoc -Iapi/proto --python_out=. --grpc_python_out=. api/proto/beastdb.proto
`

### Usage
`python
import grpc
import beastdb_pb2
import beastdb_pb2_grpc

def main():
    # Connect to BeastDB Leader
    channel = grpc.insecure_channel(localhost:50051)
    client = beastdb_pb2_grpc.BeastDBServiceStub(channel)

    # Put a key-value pair
    put_res = client.Put(beastdb_pb2.PutRequest(key=42, value=bPython powered!))
    print(fPut success: {put_res.success})

    # Point read
    get_res = client.Get(beastdb_pb2.GetRequest(key=42))
    if get_res.found:
        print(fFound Key {get_res.key}: {get_res.value.decode('utf-8')})

    # Streaming range scan
    scan_stream = client.Scan(beastdb_pb2.ScanRequest(start_key=1, end_key=100))
    for record in scan_stream:
        print(fKey: {record.key} -> {record.value.decode('utf-8')})

if __name__ == __main__:
    main()
`

---

## 3. Node.js / TypeScript Client

### Setup
`ash
npm install @grpc/grpc-js @grpc/proto-loader
`

### Usage
`javascript
const grpc = require('@grpc/grpc-js');
const protoLoader = require('@grpc/proto-loader');
const path = require('path');

const PROTO_PATH = path.join(__dirname, '../api/proto/beastdb.proto');
const packageDef = protoLoader.loadSync(PROTO_PATH, {
  keepCase: true,
  longs: String,
  enums: String,
  defaults: true,
  oneofs: true,
});

const proto = grpc.loadPackageDefinition(packageDef).beastdb.v1;
const client = new proto.BeastDBService(
  'localhost:50051',
  grpc.credentials.createInsecure()
);

// Write key-value
client.Put({ key: 42, value: Buffer.from('Node.js client') }, (err, res) => {
  if (err) console.error(err);
  console.log('Put status:', res.success);

  // Read key-value
  client.Get({ key: 42 }, (err, res) => {
    if (res.found) {
      console.log(Key :, res.value.toString('utf-8'));
    }
  });
});
`

---

## 4. Go Client

`go
package main

import (
	context
	fmt
	log

	beastv1 github.com/ChromaBeast/beastdb/api/proto
	google.golang.org/grpc
	google.golang.org/grpc/credentials/insecure
)

func main() {
	conn, err := grpc.NewClient(localhost:50051, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf(Failed to connect: %v, err)
	}
	defer conn.Close()

	client := beastv1.NewBeastDBServiceClient(conn)
	ctx := context.Background()

	// Put
	client.Put(ctx, &beastv1.PutRequest{Key: 42, Value: []byte(Go SDK)})

	// Get
	resp, _ := client.Get(ctx, &beastv1.GetRequest{Key: 42})
	fmt.Printf(Key 42: %s (Found=%v)\n, string(resp.Value), resp.Found)
}
`

---

## 5. Summary

| Language | Client Mechanism | Runtime Dependency |
|---|---|---|
| **Python** | grpcio + generated stubs | None (Go not needed) |
| **Node / TS** | @grpc/grpc-js + dynamic/static proto | None (Go not needed) |
| **Rust** | 	onic + prost | None (Go not needed) |
| **Java** | grpc-java + protobuf-java | None (Go not needed) |
| **C# / .NET** | Grpc.Net.Client | None (Go not needed) |

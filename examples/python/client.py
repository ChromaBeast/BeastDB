"
BeastDB Python Client Example
Connects to BeastDB gRPC server, performs Put, Get, and streaming Scan operations.
Prerequisites:
    pip install grpcio grpcio-tools protobuf
    python -m grpc_tools.protoc -I../../api/proto --python_out=. --grpc_python_out=. ../../api/proto/beastdb.proto
"

import sys
import grpc

try:
    import beastdb_pb2
    import beastdb_pb2_grpc
except ImportError:
    print([!] Protobuf stubs not found. Generate them with:)
    print( python -m grpc_tools.protoc -I../../api/proto --python_out=. --grpc_python_out=. ../../api/proto/beastdb.proto)
    sys.exit(1)


def run(target: str = localhost:50051):
    print(fConnecting to BeastDB cluster at {target}...)
    with grpc.insecure_channel(target) as channel:
        client = beastdb_pb2_grpc.BeastDBServiceStub(channel)

        # 1. Put key-value
        print([+] Writing key 101...)
        put_res = client.Put(beastdb_pb2.PutRequest(key=101, value=bbeast_mode_python))
        print(f Put success: {put_res.success})

        # 2. Get key-value
        print([+] Reading key 101...)
        get_res = client.Get(beastdb_pb2.GetRequest(key=101))
        if get_res.found:
            print(f Read back: {get_res.value.decode('utf-8')})
        else:
            print( Key not found!)

        # 3. Stream range scan
        print([+] Performing range scan [100, 200]...)
        scan_stream = client.Scan(beastdb_pb2.ScanRequest(start_key=100, end_key=200))
        for item in scan_stream:
            print(f Record: key={item.key}, val={item.value.decode('utf-8')})


if __name__ == __main__:
    addr = sys.argv[1] if len(sys.argv) > 1 else localhost:50051
    run(addr)

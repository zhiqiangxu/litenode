package main

import (
	"fmt"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"github.com/zhiqiangxu/litenode"
	"github.com/zhiqiangxu/litenode/eth"
	common2 "github.com/zhiqiangxu/litenode/eth/common"
)

func main() {
	zeroLogInit()

	config := litenode.Config{
		Eth: &common2.NodeConfig{
			P2P: p2p.Config{
				MaxPeers: 999,
				BootstrapNodes: eth.Nodes{
					"enode://1cc4534b14cfe351ab740a1418ab944a234ca2f702915eadb7e558a02010cb7c5a8c295a3b56bcefa7701c07752acd5539cb13df2aab8ae2d98934d712611443@52.71.43.172:30311", "enode://28b1d16562dac280dacaaf45d54516b85bc6c994252a9825c5cc4e080d3e53446d05f63ba495ea7d44d6c316b54cd92b245c5c328c37da24605c4a93a0d099c4@34.246.65.14:30311", "enode://5a7b996048d1b0a07683a949662c87c09b55247ce774aeee10bb886892e586e3c604564393292e38ef43c023ee9981e1f8b335766ec4f0f256e57f8640b079d5@35.73.137.11:30311",
				}.Convert(),
				StaticNodes: eth.Nodes{
					"enode://9f90d69c5fef1ca0b1417a1423038aa493a7f12d8e3d27e10a5a8fd3da216e485cf6c15f48ee310a14729bc3a4b05038479476c0aa82eed3c5d9d2e64ba3a2b3@52.69.42.169:30311", "enode://78ef719ebb2f4fc222aa988a356274dcd3624fb808936ca2ea77388ca229773d4351f795abf505e86db1a30ed1523ded9f9674d916b295bfb98516b78d2844be@13.231.200.147:30311", "enode://a8ff9670029785a644fb709ec7cd7e7e2d2b93761872bfe1b011a1ed1c601b23ffa69ead0901b759d780ed65aa81444261905b6964bdf8647bf5b061a4796d2d@54.168.191.244:30311", "enode://0f0abad52d6e3099776f70fda913611ad33c9f4b7cafad6595691ea1dd57a37804738be65315fc417d41ab52632c55a5f5f1e5ed3123ed64a312341a8c3f9e3c@52.193.230.222:30311", "enode://ecc277f466f35b249b62de8ca567dfe759162ffecc79f40339655537ee58132aec892bc0c4ad3dfb0ba5441bb7a68301c0c09e3f66454110c2c03ccca084c6b5@54.238.240.9:30311", "enode://dd3fb5f4da631067d0a9206bb0ac4400d3a076102194257911b632c5aa56f6a3289a855cc0960ad7f2cda3ba5162e0d879448775b07fa73ccd2e4e0477290d9a@54.199.96.72:30311", "enode://74481dd5079320755588b5243f82ddec7364ad36108ac77272b8e003194bb3f5e6386fcd5e50a0604db1032ac8cb9b58bb813f8e57125ad84ec6ceec65d29b4b@52.192.99.35:30311", "enode://190df80c16509d9d205145495f169a605d1459e270558f9684fcd7376934e43c65a38999d5e49d2ad118f49abfb6ff62068051ce49acc029da7d2be9910fe9fd@13.113.113.139:30311", "enode://368fc439d8f86f459822f67d9e8d1984bab32098096dc13d4d361f8a4eaf8362caae3af86e6b31524bda9e46910ac61b075728b14af163eca45413421386b7e2@52.68.165.102:30311", "enode://2038dac8d835db7c4c1f9d2647e37e6f5c5dc5474853899adb9b61700e575d237156539a720ff53cdb182ee56ac381698f357c7811f8eadc56858e0d141dcce0@18.182.11.67:30311", "enode://fc0bb7f6fc79ad7d867332073218753cb9fe5687764925f8405459a98b30f8e39d4da3a10f87fe06aa10df426c2c24c3907a4d81df4e3c88e890f7de8f8980de@54.65.239.152:30311", "enode://3aaaa0e0c7961ef3a9bf05f879f84308ca59651327cf94b64252f67448e582dcd6a6dbe996264367c8aa27fc302736db0283a3516c7406d48f268c5e317b9d49@34.250.1.192:30311", "enode://62c516645635f0389b4c851bfc4545720fac0607de74942e4ea7e923f4fa2ac0c438c146e2f0721c8ce06dca4e7f30f5c0136569d9f4b6a827c62b980fd53272@52.215.57.20:30311", "enode://5df2f71ae6b2e3bb92f92badbce1f601feabd2d6ce899cf8265c39c38ff446136d74f5bfa089532c7074bb7606a509a54a2ac66397aaaab2363dad3f43c687a8@79.125.103.83:30311", "enode://760b5fde9bc14155fa2a87e56cf610701ad6c1adcf44555a7b839baf71f86f11cdadcaf925e50b17c98cc28e20e0df3c3463caad7c6658a76ab68389af639f33@34.243.1.225:30311",
				}.Convert(),
				TrustedNodes: eth.Nodes{}.Convert(),
				// EnableMsgEvents: true,
			},
			Handler: common2.HandlerConfig{
				NetworkID:   56,
				GenesisHash: common.HexToHash("0x0d21840abff46b96c84b2ac9e10e4f5cdaeb5693cb665db62a2f3b02d2d57b5b"),
				Upgrade:     true,
			},
			TxPool:   common2.TxPoolConfig{HashCap: 10000, TxCap: 1000},
			LogLevel: log.LvlInfo,
			EthProtocolVersions: common2.ProtocolVersions{
				Versions: []uint{common2.ETH67, common2.ETH66, common2.ETH65},
				Lengths:  map[uint]uint64{common2.ETH67: 18, common2.ETH66: 17, common2.ETH65: 17},
			},
			SnapProtocolVersions: &common2.ProtocolVersions{
				Versions: []uint{common2.SNAP1},
				Lengths:  map[uint]uint64{common2.SNAP1: 8},
			},
		},
	}
	node := litenode.New(config)

	err := node.Start()
	if err != nil {
		panic(err)
	}

	go func() {
		ch := make(chan common2.SnapSyncPacket)
		sub := node.Eth.SubscribeSnapSyncMsg(ch)
		defer sub.Unsubscribe()

		for {
			packet := <-ch
			fmt.Println("snap packet", packet.Packet)
		}
	}()
	for {
		zlog.Info().Int("peers", node.Eth.PeerCount()).Msg("PeerCount")
		time.Sleep(time.Second)
	}
}

func consoleWriter() zerolog.ConsoleWriter {
	cw := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339Nano,
	}
	return cw
}

func zeroLogInit() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMicro
	zlog.Logger = zerolog.New(consoleWriter()).With().Timestamp().Logger()
}

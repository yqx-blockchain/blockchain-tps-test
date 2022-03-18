package main

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/tak1827/blockchain-tps-test/tps"
)

func main_e() {

	var (
		Endpoint = "http://127.0.0.1:8545" // testnet

		Timeout          = 15 * time.Second
		MaxConcurrency   = runtime.NumCPU()
		mesuringDuration = 60 * time.Second //执行数据时间
		queueSize        = 300              //队列大小
		concurrency      = 150              //并发数量
		queue            = tps.NewQueue(queueSize)
		closing          uint32
		tpsClosing       uint32
		idlingDuration   uint32
		logLevel         = tps.WARN_LEVEL // INFO_LEVEL, WARN_LEVEL, FATAL_LEVEL
		logger           = tps.NewLogger(logLevel)
		privs            = []string{
			"私钥",
		}
	)

	go func() {
		defer atomic.AddUint32(&closing, 1)
		time.Sleep(mesuringDuration)
	}()

	go func() {
		defer atomic.AddUint32(&tpsClosing, 1)
		time.Sleep(mesuringDuration * 2)
	}()

	client, err := NewClient(Endpoint)
	if err != nil {
		logger.Fatal("err NewClient: ", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	addrs := make([]string, len(privs))
	for i := range privs {
		fromAddress := generateAddress(generatePublicKey(privateKeyHextoECDSA(privs[i])))
		addrs[i] = fromAddress.Hex()
	}

	wallet, err := tps.NewWallet(ctx, &client, privs, addrs)
	if err != nil {
		logger.Fatal("err NewWallet: ", err)
	}

	taskDo := func(t tps.Task, id int) error {
		task, ok := t.(*EthTask)
		if !ok {
			return errors.New("unexpected task type")
		}

		ctx, cancel := context.WithTimeout(context.Background(), Timeout)
		defer cancel()

		var (
			priv         = wallet.Priv(id)
			currentNonce = wallet.IncrementNonce(priv)
		)
		if err = task.Do(ctx, &client, priv, currentNonce, &queue, logger, ""); err != nil {
			if errors.Is(err, tps.ErrWrongNonce) {
				wallet.RecetNonce(priv, currentNonce)
				task.tryCount = 0
				queue.Push(task)
				return nil
			}
			return errors.Wrap(err, "err Do")
		}

		return nil
	}

	worker := tps.NewWorker(taskDo)

	// performance likely not improved, whene exceed available cpu core
	if concurrency > MaxConcurrency {
		logger.Warn(fmt.Sprintf("concurrency setting is over logical max(%d)", MaxConcurrency))
	}
	for i := 0; i < concurrency; i++ {
		go worker.Run(&queue, i)
	}

	go func() {
		count := 0
		for {
			if atomic.LoadUint32(&closing) == 1 {
				break
			}

			if queue.CountTasks() > queueSize {
				continue
			}

			queue.Push(&EthTask{
				to:     "0x27F6F1bb3e2977c3CB014e7d4B5639bB133A6032",
				amount: 1000000000000000, //设置打多少币 0.001
			})
			count++
		}
	}()

	if err = tps.StartTPSMeasuring(context.Background(), &client, &tpsClosing, &idlingDuration, logger); err != nil {
		fmt.Println("err StartTPSMeasuring:", err)
		logger.Fatal("err StartTPSMeasuring: ", err)
	}

	time.Sleep(10 * time.Second)

}

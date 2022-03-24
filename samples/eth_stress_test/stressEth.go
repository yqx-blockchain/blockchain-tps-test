package main

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/tak1827/blockchain-tps-test/tps"
)

func ethStressTest(client *EthClient, ctx context.Context) {

	addrs := make([]string, len(privs))
	for i := range privs {
		fromAddress := generateAddress(generatePublicKey(privateKeyHextoECDSA(privs[i])))
		addrs[i] = fromAddress.Hex()
	}

	wallet, err := tps.NewWallet(ctx, client, privs, addrs)
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
		if err = task.Do(ctx, client, priv, currentNonce, &queue, logger, ""); err != nil {
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
}

package submodelrepository

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	persistence_postgresql "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	submodel_persistence "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/Submodel"
)

const (
	benchmarkEnabled = false
)

func TestNewSubmodelHandler(smDatabase *persistence_postgresql.PostgreSQLSubmodelDatabase) {
	//TEST
	if benchmarkEnabled {
		var acc int64
		for i := 0; i < 1000; i++ {
			start := time.Now().Local().UnixMilli()
			submodel_persistence.GetSubmodelById(smDatabase.GetDB(), fmt.Sprintf("5_%d", i))
			end := time.Now().Local().UnixMilli()
			fmt.Printf("Total time: %d milliseconds\n", end-start)
			acc += int64(end - start)
		}
		fmt.Printf("Average time: %d milliseconds\n", acc/1000)
		fmt.Println("Total accumulated time:", acc)

		// Same as above but Parallel
		var wg sync.WaitGroup
		threadCount := 32
		iterations := 1024
		perThread := iterations / threadCount

		wg.Add(threadCount)
		startTime := time.Now().UnixMilli()
		for t := 0; t < threadCount; t++ {
			go func(threadID int) {
				defer wg.Done()
				localAcc := int64(0)
				startIdx := threadID * perThread
				endIdx := startIdx + perThread

				for i := startIdx; i < endIdx; i++ {
					start := time.Now().UnixMilli()
					submodel_persistence.GetSubmodelById(smDatabase.GetDB(), fmt.Sprintf("5_%d", i))
					end := time.Now().UnixMilli()
					duration := end - start
					//fmt.Printf("[Thread %02d] Total time for 5_%d: %d ms\n", threadID, i, duration)
					localAcc += duration
				}

			}(t)
		}

		wg.Wait()
		endTime := time.Now().UnixMilli()
		totalDuration := endTime - startTime
		averageDuration := totalDuration / int64(iterations)
		fmt.Printf("Parallel Execution - Total time: %d ms, Average time per request: %d ms\n", totalDuration, averageDuration)
		// Requests per second
		requestsPerSecond := float64(iterations) / (float64(totalDuration) / 1000.0)
		fmt.Printf("Requests per second: %.2f\n", requestsPerSecond)

		// sm, err := smDatabase.GetSubmodelById("5_1")
		// jsonSubmodel, _ := json.Marshal(sm)
		// fmt.Println(string(jsonSubmodel))
	}
	osData, err := os.ReadFile("aas_query.json")
	if err != nil {
		log.Fatalf("Failed to read file: %v", err)
	}
	queryString := string(osData)

	var query grammar.QueryWrapper
	err = json.Unmarshal([]byte(queryString), &query)
	if err != nil {
		log.Fatalf("Failed to parse JSON: %v", err)
	}
	start := time.Now()
	sms, cursor, err := submodel_persistence.GetAllSubmodels(smDatabase.GetDB(), 5, "", &query)
	end := time.Now()
	fmt.Printf("Query Execution Time: %d milliseconds\n", end.Sub(start).Milliseconds())
	fmt.Println(cursor)
	if err != nil {
		log.Fatalf("Failed to execute query: %v", err)
	}
	// print size in MB of result

	fmt.Println(len(sms))
	if len(sms) > 0 {
		jsonSubmodel, _ := json.Marshal(sms[0])
		//print size in bytes
		fmt.Println(string(jsonSubmodel))

		allSmsJson, _ := json.Marshal(sms)
		fmt.Printf("Total size of all submodels: %.2f MB\n", float64(len(allSmsJson))/(1024*1024))
	}
	//TEST
}

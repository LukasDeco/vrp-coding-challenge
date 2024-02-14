package main

import (
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
)

type Point struct {
	X float64
	Y float64
}

type LoadPoint struct {
	LoadNumber int
	Pickup     Point
	Dropoff    Point
}

var depot = Point{
	X: 0,
	Y: 0,
}

type Driver struct {
	minutes        float64
	assignedRoutes []int
	lastPoint      Point
}
type PotentialRoute struct {
	minutes float64
	driver  Driver
}

func (d *Driver) AddMinutes(m float64) *Driver {
	d.minutes += m
	return d
}

func (d *Driver) SetLastPoint(p Point) *Driver {
	d.lastPoint = p
	return d
}

func (d *Driver) AssignRoute(r int) *Driver {
	d.assignedRoutes = append(d.assignedRoutes, r)
	return d
}

func (lp *LoadPoint) Distance() float64 {
	return CalculateDistanceBetweenPoints(lp.Pickup, lp.Dropoff)
}

func CalculateDistanceBetweenPoints(p1, p2 Point) float64 {
	distance := math.Sqrt(math.Pow(p2.X-p1.X, 2) + math.Pow(p2.Y-p1.Y, 2))
	return distance
}

type PossibleRoutesSchedule struct {
	routes [][]int
	cost   float64
}

func main() {
	if len(os.Args) < 2 {
		panic(fmt.Errorf("no file path provided"))
	}

	filePath := os.Args[1]

	file, err := os.Open(filePath)
	if err != nil {
		panic(fmt.Errorf("error opening file: %v", err))
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		panic(fmt.Errorf("error reading file: %v", err))
	}

	loadPoints, err := parseData(data)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	var wg sync.WaitGroup
	possibleSchedules := make([]PossibleRoutesSchedule, 40)
	minCost := math.MaxFloat64
	var minCostIndex int

	for i := range possibleSchedules {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			loadPointsCopied := make([]LoadPoint, len(loadPoints))
			copy(loadPointsCopied, loadPoints)
			randomizedLoadPoints := randomizeLoadPoints(loadPointsCopied)

			routes, cost := DetermineOptimalRoutingWithCost(randomizedLoadPoints)

			possibleSchedules[index] = PossibleRoutesSchedule{
				routes: routes,
				cost:   cost,
			}

		}(i)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	// Find the schedule with the minimum cost
	for i, schedule := range possibleSchedules {
		if schedule.cost < minCost {
			minCost = schedule.cost
			minCostIndex = i
		}
	}

	logSlices(possibleSchedules[minCostIndex].routes)
}

func randomizeLoadPoints(loadPoints []LoadPoint) []LoadPoint {
	rand.Shuffle(len(loadPoints), func(i, j int) {
		loadPoints[i], loadPoints[j] = loadPoints[j], loadPoints[i]
	})
	return loadPoints
}

func (d *Driver) CanPickupLoad(l LoadPoint) bool {

	timeToPickup := CalculateDistanceBetweenPoints(d.lastPoint, l.Pickup)
	timeToGetHome := CalculateDistanceBetweenPoints(l.Dropoff, depot)
	totalAddlTime := timeToPickup + timeToGetHome + l.Distance()
	return d.minutes+totalAddlTime < 12*60

}

func shouldAddNewDriver(lp LoadPoint, preExistingPickupTimes []PotentialRoute) bool {
	// what is current cost of best existing driver picking up load
	var preExistingPickupTimeMinutes []float64
	for _, dt := range preExistingPickupTimes {
		preExistingPickupTimeMinutes = append(preExistingPickupTimeMinutes, dt.minutes)
	}
	minDriveTime := minInSlice(preExistingPickupTimeMinutes)
	currentDriverCost := minDriveTime + CalculateDistanceBetweenPoints(depot, lp.Dropoff)

	// cost added of new driver
	newDriverTime := CalculateDistanceBetweenPoints(depot, lp.Pickup)
	timeAddedFromNewDriver := newDriverTime + CalculateDistanceBetweenPoints(depot, lp.Dropoff)
	costForNewDriver := 500 + timeAddedFromNewDriver

	// determine whether it makes sense to add a new driver or not
	return costForNewDriver < currentDriverCost || len(preExistingPickupTimes) == 0
}

func DetermineOptimalRoutingWithCost(loadPoints []LoadPoint) ([][]int, float64) {
	var drivers []Driver
	for _, lp := range loadPoints {
		var pickupTimes []PotentialRoute
		for _, d := range drivers {
			if !d.CanPickupLoad(lp) {
				// skip driver who can't pickup load
				continue
			}
			var timeToDrive float64
			timeToDrive = CalculateDistanceBetweenPoints(d.lastPoint, lp.Pickup)
			pr := PotentialRoute{
				driver:  d,
				minutes: timeToDrive,
			}
			pickupTimes = append(pickupTimes, pr)
		}

		var chosenDriver *Driver
		if shouldAddNewDriver(lp, pickupTimes) {
			// adding new driver
			newDriver := Driver{}
			pr := PotentialRoute{
				driver:  newDriver,
				minutes: CalculateDistanceBetweenPoints(depot, lp.Pickup),
			}
			chosenDriver = &pr.driver
		}

		var minPickupTime float64 = 0
		if chosenDriver == nil {
			//figure out which pre-existing drivers to use based on minimum drive time
			var pickupTimeMinutes []float64
			for _, pt := range pickupTimes {
				pickupTimeMinutes = append(pickupTimeMinutes, pt.minutes)
			}
			if len(pickupTimeMinutes) > 0 {
				minPickupTime = minInSlice(pickupTimeMinutes)
			}
			for _, pt := range pickupTimes {
				if minPickupTime == pt.minutes {
					pickupTimeDriver := pt.driver
					chosenDriver = &pickupTimeDriver
				}
			}
		}

		if chosenDriver == nil {
			panic("no driver chosen")
		}

		// updating our driver
		chosenDriver = chosenDriver.AssignRoute(lp.LoadNumber)
		if chosenDriver.lastPoint == depot {
			// if it's a new driver, we have to append
			chosenDriver = chosenDriver.AddMinutes(CalculateDistanceBetweenPoints(depot, lp.Pickup))
			chosenDriver = chosenDriver.AddMinutes(lp.Distance())
			chosenDriver = chosenDriver.SetLastPoint(lp.Dropoff)
			drivers = append(drivers, *chosenDriver)
		} else {
			// pre-existing driver just set their last point and add minutes from the min drive time
			// find the driver index based on minutes
			var driverIndex int
			for i, d := range drivers {
				if d.minutes == chosenDriver.minutes {
					driverIndex = i
				}
			}
			// add pickup time + distance
			chosenDriver = chosenDriver.AddMinutes(minPickupTime + lp.Distance())
			chosenDriver.SetLastPoint(lp.Dropoff)
			drivers[driverIndex] = *chosenDriver

		}

	}

	var routes [][]int = make([][]int, 0)
	var minutesDriven float64
	for _, dt := range drivers {
		driverRoutes := make([]int, 0)
		for _, r := range dt.assignedRoutes {
			driverRoutes = append(driverRoutes, r)
		}
		routes = append(routes, driverRoutes)
		minutesDriven += dt.minutes + CalculateDistanceBetweenPoints(dt.lastPoint, depot)
	}

	driversCost := (float64(500) * float64(len(drivers)))
	cost := minutesDriven + driversCost

	return routes, cost
}

func parseData(data []byte) ([]LoadPoint, error) {
	var result []LoadPoint
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		if lines[0] == line {
			//skip the header line
			continue
		}

		// Split the line into fields
		fields := strings.Fields(line)

		if len(fields) != 3 {
			return nil, fmt.Errorf("invalid line format: %s", line)
		}

		// Parse load number
		loadNumber, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, fmt.Errorf("error parsing load number: %v", err)
		}

		// Parse pickup and dropoff points
		pickup, err := parsePoint(fields[1])
		if err != nil {
			return nil, fmt.Errorf("error parsing pickup point: %v", err)
		}

		dropoff, err := parsePoint(fields[2])
		if err != nil {
			return nil, fmt.Errorf("error parsing dropoff point: %v", err)
		}

		result = append(result, LoadPoint{
			LoadNumber: loadNumber,
			Pickup:     pickup,
			Dropoff:    dropoff,
		})
	}

	return result, nil
}

func parsePoint(s string) (Point, error) {
	s = strings.Trim(s, "()")
	coords := strings.Split(s, ",")
	if len(coords) != 2 {
		return Point{}, fmt.Errorf("invalid point format: %s", s)
	}

	x, err := strconv.ParseFloat(coords[0], 64)
	if err != nil {
		return Point{}, fmt.Errorf("error parsing x coordinate: %v", err)
	}

	y, err := strconv.ParseFloat(coords[1], 64)
	if err != nil {
		return Point{}, fmt.Errorf("error parsing y coordinate: %v", err)
	}

	return Point{X: x, Y: y}, nil
}

func minInSlice(slice []float64) float64 {
	if len(slice) == 0 {
		return math.MaxFloat64
	}

	min := slice[0]
	for _, value := range slice[1:] {
		if value < min {
			min = value
		}
	}
	return min
}

func logSlices(slices [][]int) {
	for _, slice := range slices {
		fmt.Print("[")
		for i, num := range slice {
			fmt.Print(num)
			if i < len(slice)-1 {
				fmt.Print(",")
			}
		}
		fmt.Println("]")
	}
}

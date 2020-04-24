/* maze.go - Simple maze generator
 * By Dirk Gates <dirk.gates@icancelli.com>
 * Copyright 2016-2020 Dirk Gates
 *
 * Revision history
 * ----------------
 * Rev 1.0 -- initial version
 * Rev 1.1 -- add solver
 * Rev 1.2 -- use solver to find best openings top & bottom
 * Rev 1.3 -- only start new paths at the corners of old paths
 * Rev 1.4 -- push mid wall openings left and down when done
 * Rev 1.5 -- add recursive, variable depth look ahead
 * Rev 1.6 -- eliminate 1x1 orphans during construction
 * Rev 2.0 -- translated to go (and converted to camelCase)
 * Rev 2.1 -- added multi-threaded generation
 * Rev 2.2 -- improved (more efficient) look ahead
 * Rev 2.3 -- added multi-threaded solving
 */
package main

import (
    "os"
    "fmt"
    "bufio"
    "flag"
    "time"
    "math/rand"
    "sync/atomic"
    "golang.org/x/crypto/ssh/terminal"
)

const (
    version      = "2.3"
    utsSignOn    = "\n" + "Maze Generation Console Utility "+ version +
                   "\n" + "Copyright (c) 2016-2020" +
                   "\n\n"
    blankLine    = "                                                  ";

    maxWidth     = 300
    maxHeight    = 100
    maxXSize     = ((maxHeight + 1)*2 + 1)
    maxYSize     = ((maxWidth  + 1)*2 + 1)

    path         = 0
    wall         = 1
    solved       = 2
    tried        = 3
    check        = 4

    up           = 1
    down         = 2
    left         = 3
    right        = 4

    noSearch     = false
    search       = true

    noUpdate     = false
    update       = true

    blank        = ' '  // ' '
    block        = 0x61 // '#'
    rightBottom  = 0x6a // '+'
    rightTop     = 0x6b // '+'
    leftTop      = 0x6c // '+'
    leftBottom   = 0x6d // '+'
    intersection = 0x6e // '+'
    horizontal   = 0x71 // '-'
    rightTee     = 0x74 // '+'
    leftTee      = 0x75 // '+'
    upTee        = 0x76 // '+'
    downTee      = 0x77 // '+'
    vertical     = 0x78 // '|'
)

type dirTable struct {
    x       int
    y       int
    heading int
}

var (
    stdDirection = [4]dirTable { { 2,  0, down },
                                 {-2,  0, up   },
                                 { 0,  2, right},
                                 { 0, -2, left } }

    outputLookup = [16]byte { blank     , vertical   , horizontal, leftBottom  ,
                              vertical  , vertical   , leftTop   , rightTee    ,
                              horizontal, rightBottom, horizontal, upTee       ,
                              rightTop  , leftTee    , downTee   , intersection }

    simpleLookup = [16]byte { ' ', '|', '-', '+',
                              '|', '|', '+', '+',
                              '-', '+', '-', '+',
                              '+', '+', '+', '+' }

    maze[maxXSize][maxYSize]  int32

    blankFlag         bool
    showFlag          bool
    viewFlag          bool
    lookFlag          bool

    width             int
    height            int
    fps               int
    updates           int
    minLen            int
    threads           int
    seed              int
    depthVal          int

    maxX, maxY        int32
    begX, endX        int32
    begY, endY        int32
    depth             int32
    delay             int32
    checkFlag         int32
    solvedFlag        int32
    mazeLen           int32
    pathLen           int32
    turnCnt           int32
    numPaths          int32
    numSolves         int32
    numThreads        int32
    numWallPush       int32
    numMazeCreated    int32
    numCheckExceeded  int32
    maxChecks         int32
    dspLength         int32
    dspNumChecks      int32
    solveLength       int32
    sumsolveLength    int32

    myStdout          *bufio.Writer
    curFunc           string
    outputName        string
    displayChan       chan struct{}
    finishChan        chan struct{}
)

func msSleep(n   int)          {; time.Sleep(time.Duration(int64(n) * 1000 * 1000)); }

func bool2int(b bool) int      {; if b      {; return 1; }; return 0; }
func min(x, y    int) int      {; if x <  y {; return x; }; return y; }
func max(x, y    int) int      {; if x >  y {; return x; }; return y; }
func nonZero(x   int) int      {; if x != 0 {; return x; }; return 1; }

func isEven(x    int) bool     {; return (x & 1) == 0; }
func isOdd( x    int) bool     {; return (x & 1) != 0; }

func setMaze(x, y, v int) int  {; return int(atomic.SwapInt32(&maze[x][y], int32(v))); }
func getMaze(x, y    int) int  {; return int(atomic.LoadInt32(&maze[x][y]));           }

func setInt( x *int32, v int)  {;            atomic.StoreInt32(x, int32(v));           }
func clrInt( x *int32)         {;            atomic.StoreInt32(x,  0);                 }
func incInt( x *int32)         {;            atomic.  AddInt32(x,  1);                 }
func decInt( x *int32)         {;            atomic.  AddInt32(x, -1);                 }
func addInt( x *int32, v int)  {;            atomic.  AddInt32(x, int32(v));           }
func getInt( x *int32)   int   {; return int(atomic. LoadInt32(x));                    }
func getBool(x *int32)   bool  {; return     atomic. LoadInt32(x) != 0;                }
func setBool(x *int32, v bool) {;            atomic.StoreInt32(x, int32(bool2int(v))); }

func putchar(c byte)           {; myStdout.WriteByte(c); }

func setPosition(x, y int)     {; fmt.Fprintf(myStdout, "\033[%d;%dH", x, y); myStdout.Flush(); }
func setLineDraw()             {; fmt.Fprintf(myStdout, "\033(0"           ); myStdout.Flush(); }
func clrLineDraw()             {; fmt.Fprintf(myStdout, "\033(B"           ); myStdout.Flush(); }
func setCursorOff()            {; fmt.Fprintf(myStdout, "\033[?25l"        ); myStdout.Flush(); }
func setCursorOn()             {; fmt.Fprintf(myStdout, "\033[?25h"        ); myStdout.Flush(); }
func clrScreen()               {; fmt.Fprintf(myStdout, "\033[2J"          ); myStdout.Flush(); }
func setSolved()               {; fmt.Fprintf(myStdout, "\033[32m\033[1m"  ); myStdout.Flush(); }
func clrSolved()               {; fmt.Fprintf(myStdout, "\033[30m\033[0m"  ); myStdout.Flush(); }
func setChecked()              {; fmt.Fprintf(myStdout, "\033[31m\033[1m"  ); myStdout.Flush(); }
func clrChecked()              {; fmt.Fprintf(myStdout, "\033[30m\033[0m"  ); myStdout.Flush(); }

// getConsoleSize returns the number of rows and columns available in the current terminal window.
// Defaults to 24 rows and 80 columns if the underlying system call fails.
func getConsoleSize() (int, int) {
    cols, rows, err := terminal.GetSize(0)
    if err != nil {
        rows = 24
        cols = 80
    }
    return rows, cols
}

// initializeMaze sets the entire maze to walls and creates a path around the perimeter to bound the maze.
// The maximum x, y values are set, and the initial x, y values are set to random values.
func initializeMaze(x, y *int) {
    clrInt(&maxChecks       )
    clrInt(&mazeLen         )
    clrInt(&numThreads      )
    clrInt(&numPaths        )
    clrInt(&numCheckExceeded)

    setInt(&maxX, 2*(height + 1) + 1)
    setInt(&maxY, 2*(width  + 1) + 1)

    for i := 1; i < getInt(&maxX) - 1; i++ {
        for j := 1; j < getInt(&maxY) - 1; j++ {
            setMaze(i, j, wall)
        }
    }
    for i := 0; i < getInt(&maxX); i++ {; setMaze(i, 0, path); setMaze(i, 2*(width  + 1), path); }
    for j := 0; j < getInt(&maxY); j++ {; setMaze(0, j, path); setMaze(2*(height + 1), j, path); }

    *x = 2*((rand.Intn(height)) + 1)   // random location
    *y = 2*((rand.Intn(width )) + 1)   // for first path

    setInt(&begX, 2)                   // these will
    setInt(&endX, 2*height)            // never change
}

// restoreMaze returns the maze to a pre-solved state by changing solved or tried cells back to paths.
func restoreMaze()  {
    for i := 0; i < getInt(&maxX); i++ {
        for j := 0; j < getInt(&maxY); j++ {
            if (getMaze(i, j) == solved ||
                getMaze(i, j) == tried) {
                setMaze(i, j,    path )
            }
        }
    }
}

// outputAsciiMaze outputs the maze in ascii format to a text file
func outputAsciiMaze() {
    if outputName != "" {
        f, err := os.Create(outputName)
        if err != nil {
            fmt.Fprintf(myStdout, "Error opening output file: ", err)
            myStdout.Flush()
        } else {
            outFile := bufio.NewWriterSize(f, getInt(&maxX) * getInt(&maxY))
            fmt.Fprintf(outFile, "%d %d\n", height, width)
            outFile.Flush()
            for i := 1; i < getInt(&maxX) - 1; i++ {
                for j := 1; j < getInt(&maxY) - 1; j++ {
                    switch getMaze(i, j) {
                        case wall  : if isOdd(i) && isOdd(j) {; fmt.Fprintf(outFile, "%c", simpleLookup[1 * bool2int(getMaze(i-1, j) == wall && (getMaze(i-1, j-1) != wall || getMaze(i-1, j+1) != wall)) +    // wall intersection point
                                                                                                        2 * bool2int(getMaze(i, j+1) == wall && (getMaze(i-1, j+1) != wall || getMaze(i+1, j+1) != wall)) +    // check that there is a path on the diagonal
                                                                                                        4 * bool2int(getMaze(i+1, j) == wall && (getMaze(i+1, j-1) != wall || getMaze(i+1, j+1) != wall)) +
                                                                                                        8 * bool2int(getMaze(i, j-1) == wall && (getMaze(i-1, j-1) != wall || getMaze(i+1, j-1) != wall))])
                                     } else if      isOdd(i) {; fmt.Fprintf(outFile, "-")
                                     } else {                 ; fmt.Fprintf(outFile, "|"); }
                        case path  :                            fmt.Fprintf(outFile, " ")
                        case tried :                            fmt.Fprintf(outFile, ".")
                        case solved:                            fmt.Fprintf(outFile, "*")
                        case check :                            fmt.Fprintf(outFile, "#")
                        default    :                            fmt.Fprintf(outFile, "?")
                    }
                }
                fmt.Fprintf(outFile, "\n")
                outFile.Flush()
            }
            f.Close()
        }
    }
}

// isWall returns true if a cell contains a wall character or a check character (to hide look ahead checks during display)
func isWall(cell int) bool {
    return cell == wall || (!getBool(&checkFlag) && cell == check)
}

// displayMaze displays the current maze within the terminal window using VT100 line drawing characters.
func displayMaze()  {
    setPosition(0, 0)
    setLineDraw()

    for i := 1; i < getInt(&maxX) - 1; i++ {
        for j := 1; j < getInt(&maxY) - 1; j++ {
            var vertexChar, solvedChar, leftChar, rightChar, wallChar byte

            if isOdd(i) && isOdd(j) {
                vertexChar = outputLookup[1 * bool2int(isWall(getMaze(i-1, j)) && (!isWall(getMaze(i-1, j-1)) || !isWall(getMaze(i-1, j+1)))) +    // wall intersection point
                                          2 * bool2int(isWall(getMaze(i, j+1)) && (!isWall(getMaze(i-1, j+1)) || !isWall(getMaze(i+1, j+1)))) +    // check that there is a path on the diagonal
                                          4 * bool2int(isWall(getMaze(i+1, j)) && (!isWall(getMaze(i+1, j-1)) || !isWall(getMaze(i+1, j+1)))) +
                                          8 * bool2int(isWall(getMaze(i, j-1)) && (!isWall(getMaze(i-1, j-1)) || !isWall(getMaze(i+1, j-1))))];
            } else {
                vertexChar = outputLookup[1 * bool2int(isWall(getMaze(i-1, j)) && (!isWall(getMaze(i  , j-1)) || !isWall(getMaze(i  , j+1)))) +    // non-intersection point
                                          2 * bool2int(isWall(getMaze(i, j+1)) && (!isWall(getMaze(i-1, j  )) || !isWall(getMaze(i+1, j  )))) +    // check that there is a path adjacent
                                          4 * bool2int(isWall(getMaze(i+1, j)) && (!isWall(getMaze(i  , j-1)) || !isWall(getMaze(i  , j+1)))) +
                                          8 * bool2int(isWall(getMaze(i, j-1)) && (!isWall(getMaze(i-1, j  )) || !isWall(getMaze(i+1, j  ))))];
            }
                solvedChar = outputLookup[1 * bool2int(getMaze(i-1, j) == getMaze(i, j)) +
                                          2 * bool2int(getMaze(i, j+1) == getMaze(i, j)) +
                                          4 * bool2int(getMaze(i+1, j) == getMaze(i, j)) +
                                          8 * bool2int(getMaze(i, j-1) == getMaze(i, j))]

            if isEven(i) && (getMaze(i, j-1) == solved || getMaze(i, j-1) == check) {;  leftChar = horizontal; } else {;  leftChar = blank; }
            if isEven(i) && (getMaze(i, j+1) == solved || getMaze(i, j+1) == check) {; rightChar = horizontal; } else {; rightChar = blank; }

            if blankFlag {; wallChar = vertexChar; } else {; wallChar = solvedChar; }

            switch {
                case getMaze(i, j) == solved:                           setSolved();  putchar(leftChar); if (isEven(j)) {; putchar(solvedChar); putchar(rightChar); }; clrSolved()
                case getMaze(i, j) == check : if getBool(&checkFlag) {; setChecked(); putchar(leftChar); if (isEven(j)) {; putchar(solvedChar); putchar(rightChar); }; clrChecked();
                                              } else                 {;               putchar(blank   ); if (isEven(j)) {; putchar(blank     ); putchar(blank    ); }}
                case isEven(i) && isEven(j) :                                         putchar(blank   ); if (isEven(j)) {; putchar(blank     ); putchar(blank    ); }
                case getMaze(i, j) == wall  :                                         putchar(wallChar); if (isEven(j)) {; putchar(wallChar  ); putchar(wallChar ); }
                default                     :                                         putchar(blank   ); if (isEven(j)) {; putchar(blank     ); putchar(blank    ); }
            }
        }
        putchar('\n')
    }
    clrLineDraw()
    updates++;

    fmt.Fprintf(myStdout, "updates=%d, height=%d, width=%d, seed=%d, num_wall_push=%d, num_maze_created=%d, num_solves=%d, avg_solve_length=%d, solve_length=%d, avg_path_length=%d, num_paths=%d, maze_len=%d, threads=%d, length=%d, checks=%d, max_checks=%d, checks_exceeded=%d %s\r",
                           updates   , height   , width   , seed   ,
                           getInt(&numWallPush     ),
                           getInt(&numMazeCreated  ),
                           getInt(&numSolves       ),
                           getInt(&sumsolveLength  ) /
                   nonZero(getInt(&numMazeCreated  )),
                           getInt(&solveLength     ),
                           getInt(&mazeLen         ) /
                   nonZero(getInt(&numPaths        )),
                           getInt(&numPaths        ),
                           getInt(&mazeLen         ),
                           getInt(&numThreads      ),
                           getInt(&dspLength       ),
                           getInt(&dspNumChecks    ),
                           getInt(&maxChecks       ),
                           getInt(&numCheckExceeded),
                           blankLine);
    outputAsciiMaze()
}

// displayRoutine waits to receive a signal on displayChan and then prints the maze
func displayRoutine () {
    for range displayChan {
        displayMaze()
    }
}

// updateMaze signals displayChan if there is no pending signal and then sleeps delay ms if non-zero
func updateMaze(numChecks int) {
    if numChecks != 0 {
        setInt(&dspNumChecks, numChecks)
    }
    select {
        case displayChan <- struct{}{}:
        default:
    }
    if getInt(&delay) > 0 {
        msSleep(getInt(&delay))
    }
}

// setCell sets a location x, y inside the maze array to the value (wall, path, solved, tried)
// It also displays the maze if delay is non-zero and the frame rate is less than 1000/sec
// and only then for cells at locations with even x, y coordinates (to reduce number of refreshes)
func setCell(x, y, value int, update bool, length, numChecks int) bool {
    if getMaze(x, y) == check || getMaze(x, y) == value {
        return false
    }
    priorValue := setMaze(x, y, value)
    if priorValue  == check {
        setMaze(x, y, check)
        return false
    }
    if priorValue == value {
        return false
    }
    if (update || (getBool(&checkFlag) && getMaze(x, y) == check)) && getInt(&delay) > 0 && fps <= 1000 && isEven(x) && isEven(y) {
        updateMaze(numChecks)
    }
    return true
}

// checkDirections recursively checks to see if a path of a given length can be carved or traced from the given x, y location.
// (limited to a total of 1/2 million checks)
func checkDirections(x, y, dx, dy, limit, value int, length, minLength, checks, numChecks *int) bool {
    if *length < 0 {
        return true
    }
    if *checks >= limit {
        incInt(&numCheckExceeded)
        return false
    }
    if x + dx < 0 || y + dy < 0 || getMaze(x + dx, y + dy) != value || !setCell(x + dx/2, y + dy/2, check, getBool(&checkFlag), *length, *numChecks) {
        return false
    }
    if !setCell(x + dx, y + dy, check, getBool(&checkFlag), *length, *numChecks) {
        setMaze(x + dx/2, y + dy/2, value)
        return false
    }
    *length--
    *checks++
    *numChecks++
    offset := rand.Intn(4)
    match  := false
    for  i := 0; i < 4; i++ {
        dir := &stdDirection[(i + offset) % 4]
        dirLength := *length
        if getMaze(x + dx + dir.x/2, y + dy + dir.y/2) == value &&
           getMaze(x + dx + dir.x  , y + dy + dir.y  ) == value && checkDirections(x + dx, y + dy, dir.x, dir.y, limit, value, &dirLength, minLength, checks, numChecks) {
           *length = dirLength
           match = true
           break
        }
    }
    if *minLength > *length {
       *minLength = *length
    }
    *length++
    setMaze(x + dx  , y + dy  , value)
    setMaze(x + dx/2, y + dy/2, value)
    return match
}

// orphan1x1 returns true if a location is surrounded by walls on all 4 sides and paths on the other side of all those walls.
func orphan1x1(x, y int) bool {
    return         x > 1             &&         y > 1             &&  // bounds check
           getMaze(x + 1, y) == wall && getMaze(x + 2, y) == path &&  // vertical (look down & up)
           getMaze(x - 1, y) == wall && getMaze(x - 2, y) == path &&
           getMaze(x, y + 1) == wall && getMaze(x, y + 2) == path &&  // horizontal (right & left)
           getMaze(x, y - 1) == wall && getMaze(x, y - 2) == path
}

// checkOrphan returns true if carving a path at a given location x,y in a given direction dx, dy
// would create a 1x1 orphan left, right, above, or below the path.
func checkOrphan(x, y, dx, dy, length int) bool {
    orphan := false;
    if      x > 1  && y > 1   && length > 0 && length == getInt(&depth) &&  // this only makes sense when carving paths, not when solving, and only if we haven't exhausted our search depth
       getMaze(x + dx  , y + dy  ) ==  wall                             &&
       getMaze(x + dx/2, y + dy/2) ==  wall                             &&
       setCell(x + dx  , y + dy  , path, noUpdate, length, 0)           &&  // temporarily set new path
       setCell(x + dx/2, y + dy/2, path, noUpdate, length, 0) {

        orphan = orphan1x1(x + dx + 2, y + dy    ) ||   // check for 1x1 orphans below & above of the new location
                 orphan1x1(x + dx - 2, y + dy    ) ||
                 orphan1x1(x + dx    , y + dy + 2) ||   // check for 1x1 orphans right & left  of the new location
                 orphan1x1(x + dx    , y + dy - 2)

        setMaze(x + dx  , y + dy  , wall)               // restore original walls
        setMaze(x + dx/2, y + dy/2, wall)
    }
    return orphan
}

// look returns 1 if at a given location x, y a path of a given length can be carved or traced in a given direction dx, dy without creating 1x1 orphans.
// The direction (heading, dx, dy) is stored in the direction table directions if the path can be created.
func look(heading, x, y, dx, dy, num, value int, directions []dirTable, length, minLength, numChecks *int) int {
    checks := 0
    if         x > 1  && y > 1              &&
       getMaze(x + dx/2, y + dy/2) == value &&
       getMaze(x + dx  , y + dy  ) == value && !checkOrphan(x, y, dx, dy, *length) && checkDirections(x, y, dx, dy, 10*(getInt(&depth) + 1), value, length, minLength, &checks, numChecks) {
        directions[num].x = dx
        directions[num].y = dy
        directions[num].heading = heading
        return 1
    }
    return 0
}

// findDirections returns the number of directions that a path can be carved or traces from a given location x, y.
// The path length requirement of length is enforced.
func findDirections(x, y int, length *int, value int, directions []dirTable) int {
    num       := 0
    numChecks := 0
    if value != wall || (getMaze(x, y) == path && setCell(x, y, check, noUpdate, *length, numChecks)) {
        minLength := [4]int {*length, *length, *length, *length}
        len := *length
        for {
            setInt(&dspLength, len)
            dirLength := [4]int {len, len, len, len}
            offset    := rand.Intn(4)
            for i := 0; i < 4; i++ {
                dir := &stdDirection[(i + offset) % 4]
                num += look(dir.heading, x, y, dir.x, dir.y, num, value, directions, &dirLength[i] , &minLength[i], &numChecks)
            }
            if num > 0 || len < 0 {
               break
            }
            minLength := min( min(minLength[0], minLength[1]), min(minLength[2], minLength[3]) )
            if minLength <= 0 {
               minLength = 1
            }
            len -= minLength
        }
        if len == *length && len < getInt(&depth) {
           len++
        }
        *length = len
        if getMaze(x, y) == check {
           setMaze(x, y, path)
        }
    }
    if getInt(&maxChecks) < numChecks  {
       setInt(&maxChecks  , numChecks)
    }
    return (num);
}

// straightThru returns true if the path at the given location x, y has a path left and right of it, or above and below it
func straightThru(x, y, value int) bool {
    return           x > 1              &&         y > 1               &&
           ((getMaze(x - 1, y) == value && getMaze(x - 2, y) == value  &&  // vertical   (look up & down)
             getMaze(x + 1, y) == value && getMaze(x + 2, y) == value) ||
            (getMaze(x, y - 1) == value && getMaze(x, y - 2) == value  &&  // horizontal (look left & right)
             getMaze(x, y + 1) == value && getMaze(x, y + 2) == value))
}

// findPathStart starts looking at a random x, y location for a position along an existing non-straight through path that can start a new path
func findPathStart(x, y *int) bool {
    directions := make([]dirTable, 4, 4)
    xStart := rand.Intn(height)
    yStart := rand.Intn(width )
    length := -1
    for  i := 0; i < height; i++ {
        for j := 0; j < width; j++ {
            *x = 2*((xStart + i) % height + 1)
            *y = 2*((yStart + j) % width  + 1)
            if (getMaze(*x, *y) == path && !straightThru(*x, *y, path) && findDirections(*x, *y, &length, wall, directions) > 0) {
                return true
            }
        }
    }
    return false
}

// carvePath carves a new path in the maze starting at location x, y
// It does this by repeatedly determining the number of possible directions to move
// and then randomly choosing one of them and then marking the new cells on the path
func carvePath(x, y *int) bool {
    directions := make([]dirTable, 4, 4)
    length     := getInt(&depth)
    pathLength := 0
    incInt(&numPaths)
    setCell(*x, *y, path, noUpdate, 0, 0)
    for {
        num := findDirections(*x, *y, &length, wall, directions)
        if num == 0 {
           break
        }
        dir := rand.Intn(num)
        if !setCell(*x + directions[dir].x/2, *y + directions[dir].y/2, path, update, 0, 0) {
            continue
        }
        if !setCell(*x + directions[dir].x  , *y + directions[dir].y  , path, update, 0, 0) {
            setCell(*x + directions[dir].x/2, *y + directions[dir].y/2, wall, update, 0, 0)
            continue
        }
        *x += directions[dir].x
        *y += directions[dir].y
        incInt(&mazeLen)
        pathLength++
    }
    if getInt(&delay) > 0 {
        updateMaze(0)
    }
    if getInt(&numThreads) < threads {
       incInt(&numThreads)
       go carveRoutine()
    }
    return pathLength > 0
}

// followDir marks the maze solved in the given direction starting at x, y
// and updates the path length & turn count accordingly
func followDir (x, y *int, direction dirTable, lastDir int) {
    setCell(*x + direction.x/2, *y + direction.y/2, solved, update, 0, 0)
    setCell(*x + direction.x  , *y + direction.y  , solved, update, 0, 0)
    incInt(&pathLen)
    if (lastDir != direction.heading)  {
        lastDir  = direction.heading
        incInt(&turnCnt)
    }
}

// unfollowDir marks the maze tried in the given direction starting at x, y
// and updates the path length & turn count accordingly
func unfollowDir (x, y *int, direction dirTable, lastDir int) {
    setCell(*x                , *y                , tried, update, 0, 0)
    setCell(*x + direction.x/2, *y + direction.y/2, tried, update, 0, 0)
    decInt(&pathLen)
    if (lastDir != direction.heading)  {
        lastDir  = direction.heading
        decInt(&turnCnt)
    }
}

// followPath follows a path in the maze starting at location x, y
// It does this by repeatedly determining if there are any possible directions to move
// and then choosing the first of them and then marking the new cells on the path as solved
func followPath(x, y *int) bool {
    directions := make([]dirTable, 4, 4)
    lastDir    :=  0
    length     := -1
    setCell(*x, *y, solved, noUpdate, 0, 0)
    for getInt(&begX) <= *x && *x <= getInt(&endX) {
        num := findDirections(*x, *y, &length, path, directions)
        if num == 0 {
            break
        }
        followDir(x, y, directions[0], lastDir)
        if threads > 1 && num > 1 && getBool(&solvedFlag) == false {
            for i := 1; i < num; i++ {
                incInt(&numThreads)
                followDir(x, y, directions[i], lastDir)
                go solve(*x  +  directions[i].x, *y + directions[i].y)
            }
        }
        *x += directions[0].x
        *y += directions[0].y
    }
    if *x > getInt(&endX) {
        setBool(&solvedFlag, true)
        return true
    } else {
        return false
    }
}

// backTrackPath backtracks a path in the maze starting at location x, y
// It does this by repeatedly determining if there are any possible directions to move
// and then choosing the first of them and then marking the new cells on the path as tried (not solved)
func backTrackPath(x, y *int) {
    directions := make([]dirTable, 4, 4)
    lastDir    :=  0
    length     := -1
    for (threads > 1 || findDirections(*x, *y, &length, path  , directions) == 0) &&
                        findDirections(*x, *y, &length, solved, directions) == 1 {
        unfollowDir(x, y, directions[0], lastDir)
        *x += directions[0].x
        *y += directions[0].y
    }
}

// solveRoutine calls follows a single path and backtracks if it doesn't complete the maze
func solve(x, y int) {
    if   !followPath(&x, &y) {
       backTrackPath(&x, &y)
    }
    finishChan <- struct{}{}
}

// waitThreadsDone waits until numThreads signals are received on the finish channel
func waitThreadsDone() {
    for i := 0; i < getInt(&numThreads); i++ {
        <- finishChan
    }
}

// solveMaze solves a maze by starting at the beginning and following each path,
// back tracking when they dead end, until the end of the maze is found.
func solveMaze(x, y *int) {
    saveCheck := getBool(&checkFlag); setBool(&checkFlag, false)
    saveDepth := getInt( &depth    ); setInt( &depth    , -1   )
    setBool(&solvedFlag, false)
    setInt( &pathLen   , 0)
    setInt( &turnCnt   , 0)

    setMaze(getInt(&begX) - 2, getInt(&begY), solved)
    setMaze(getInt(&begX) - 1, getInt(&begY), solved)
    if threads > 1 {
        setInt(&numThreads, 1)
        go solve(*x, *y)
        waitThreadsDone()
    } else {
        for  !followPath(x, y) {
           backTrackPath(x, y)
        }
    }
    setMaze(getInt(&endX) + 1, getInt(&endY), solved)
    setMaze(getInt(&endX) + 2, getInt(&endY), solved)
    setBool(&checkFlag, saveCheck)
    setInt( &depth    , saveDepth)
}

// createOpenings marks the top and bottom of the maze at locations begX, x and endX, y as paths
// and then sets x, y to the start of the maze: begX, begY.
func createOpenings(x, y *int) {
    setInt(&begY, *x)
    setInt(&endY, *y)
    setMaze(getInt(&begX) - 1, getInt(&begY), path)
    setMaze(getInt(&endX) + 1, getInt(&endY), path)
    *x = getInt(&begX)
    *y = getInt(&begY)
}

// deleteOpenings marks the openings in the maze by setting the locations begX, begY and endX, endY back to wall.
func deleteOpenings()  {
    setMaze(getInt(&begX) - 1, getInt(&begY), wall)
    setMaze(getInt(&endX) + 1, getInt(&endY), wall)
}

// searchBestOpenings sets the top an bottom openings to all possible locations and repeatedly solves the maze
// keeping track of which set of openings produces the longest solution path, then sets x, y to the result.
func searchBestOpenings(x, y *int) {
    bestPathLen := 0
    bestTurnCnt := 0
    bestStart   := 2
    bestFinish  := 2
    saveDelay   := getInt(&delay)    // don't print updates while solving for best openings
    setInt(&delay, 0)

    for i := 0; i < width; i++ {
        for j := 0; j < width; j++ {
            start  := 2*(i + 1)
            finish := 2*(j + 1)
            *x = start
            *y = finish
            if getMaze(getInt(&begX), start  - 1) != wall && getMaze(getInt(&begX), start  + 1) != wall {; continue; }
            if getMaze(getInt(&endX), finish - 1) != wall && getMaze(getInt(&endX), finish + 1) != wall {; continue; }
            createOpenings(x, y)
            solveMaze(x, y)
            if getInt(&pathLen)  >  bestPathLen ||
              (getInt(&pathLen)  == bestPathLen &&
               getInt(&turnCnt)  >  bestTurnCnt) {
               bestStart   = start
               bestFinish  = finish
               bestTurnCnt = getInt(&turnCnt)
               bestPathLen = getInt(&pathLen)
               setInt(&solveLength, getInt(&pathLen))
            }
            restoreMaze()
            deleteOpenings()
            incInt(&numSolves)
        }
    }
    addInt(&sumsolveLength, getInt(&solveLength))
    if viewFlag {
        setInt(&delay, saveDelay)   // only restore delay value if view solve flag is set
    }
    *x = bestStart
    *y = bestFinish
    createOpenings(x, y)
}

// midWallOpening returns true if there is a mid wall (non-corner) opening in a path at location x, y
func midWallOpening(x, y int) bool {
    return        x > 0 && y > 0         &&
           getMaze(x    , y    ) == path &&
           getMaze(x - 1, y - 1) != wall &&
           getMaze(x - 1, y + 1) != wall &&
           getMaze(x + 1, y - 1) != wall &&
           getMaze(x + 1, y + 1) != wall
}

// pushMidWallOpenings loops over all locations in the maze searching for mid wall openings and pushes horizontal
// openings to the right, and vertical openings down, and then returns the number of mid wall openings moved.
func pushMidWallOpenings() {
    for {
        moves := 0
        for i := 1; i < 2 * (height + 1); i++ {
            for j := (i & 1) + 1; j < 2 * (width + 1); j += 2 {
                if (midWallOpening(i, j)) {
                    setCell(i, j, wall, noUpdate, 0, 0)
                    if isOdd(i) {; setCell(i,  j + 2, path, update, 0, 0)   // push right
                    } else {;      setCell(i + 2,  j, path, update, 0, 0)   // push down
                    }
                    moves++
                    incInt(&numWallPush)
                }
            }
        }
        if getInt(&delay) > 0 {
            updateMaze(0)
        }
        if moves == 0 {
            break
        }
    }
}

// carvePaths continuuosly carves new paths while it can find starting locations for new paths
func carvePaths(x, y int) {
    if x > 0 && y > 0 {
        carvePath(&x, &y)
    }
    for findPathStart(&x, &y) &&
            carvePath(&x, &y) {
    }
}

// carveRoutine calls carvePaths and sends a signal finshChan when complete
func carveRoutine() {
    msSleep(10)
    carvePaths(0, 0)
    finishChan <- struct{}{}
}

// createMaze initializes the maze array then repeatedly carves new paths until no new path starting locations can be found.
// Following this it then repeatedly pushes mid wall openings right or down until there are no longer any mid wall openings.
// Lastly it searches for the best openings, top and bottom, to create the maze with the longest solution path.
func createMaze(x, y *int) {
    initializeMaze(x, y)
    carvePaths(*x, *y)
    waitThreadsDone()
    pushMidWallOpenings()
    searchBestOpenings(x, y)
}

// maze main parses the command line switches and then repeatedly creates and
// solves mazes until the minimum solution path length criteria is met.
func main() {
    flag.Usage = func() {
        fmt.Printf("%s\nUsage: %s [options]\n%s", utsSignOn, flag.Arg(0),
             "Options:"                                                                                 + "\n" +
             "  -f, --fps     <frames per second>  Set refresh rate           (default: none, instant)" + "\n" +
             "  -h, --height  <height>             Set maze height            (default: screen height)" + "\n" +
             "  -w, --width   <width>              Set maze width             (default: screen width )" + "\n" +
             "  -t, --threads <threads>            Set maze path thread count (default: 0            )" + "\n" +
             "  -d, --depth   <depth>              Set path search depth      (default: 0            )" + "\n" +
             "  -p, --path    <length>             Set minimum path length    (default: 0            )" + "\n" +
             "  -r, --random  <seed>               Set random number seed     (default: current usec )" + "\n" +
             "  -s, --show                         Show intermediate results while path length not met" + "\n" +
             "  -v, --view                         Show intermediate results determining maze solution" + "\n" +
             "  -l, --look                         Show look ahead path searches while creating maze  " + "\n" +
             "  -b, --blank                        Show empty maze as blank vs. lattice work of walls " + "\n" +
             "  -o, --output  <filename>           Output portable ASCII encoded maze when completed  " + "\n\n")
    }
    rows, cols := getConsoleSize()
    maxHeight  := min(maxHeight, (rows - 3)/2)
    maxWidth   := min(maxWidth , (cols - 1)/4)
    myStdout    = bufio.NewWriterSize(os.Stdout, rows*cols)
    displayChan = make(chan struct{});
    finishChan  = make(chan struct{});

    flag.IntVar(   &fps       , "fps"    , 0        , "refresh rate"               );
    flag.IntVar(   &fps       , "f"      , 0        , "refresh rate    (shorthand)");
    flag.IntVar(   &height    , "height" , maxHeight, "maze height"                );
    flag.IntVar(   &height    , "h"      , maxHeight, "maze height     (shorthand)");
    flag.IntVar(   &width     , "width"  , maxWidth , "maze width"                 );
    flag.IntVar(   &width     , "w"      , maxWidth , "maze width      (shorthand)");
    flag.IntVar(   &threads   , "threads", 0        , "path threads"               );
    flag.IntVar(   &threads   , "t"      , 0        , "path threads    (shorthand)");
    flag.IntVar(   &depthVal  , "depth"  , 0        , "search depth"               );
    flag.IntVar(   &depthVal  , "d"      , 0        , "search depth    (shorthand)");
    flag.IntVar(   &minLen    , "path"   , 0        , "path length"                );
    flag.IntVar(   &minLen    , "p"      , 0        , "path length     (shorthand)");
    flag.IntVar(   &seed      , "random" , 0        , "random seed"                );
    flag.IntVar(   &seed      , "r"      , 0        , "random seed     (shorthand)");
    flag.BoolVar(  &showFlag  , "show"   , false    , "show working"               );
    flag.BoolVar(  &showFlag  , "s"      , false    , "show working    (shorthand)");
    flag.BoolVar(  &viewFlag  , "view"   , false    , "show solving"               );
    flag.BoolVar(  &viewFlag  , "v"      , false    , "show solving    (shorthand)");
    flag.BoolVar(  &lookFlag  , "look"   , false    , "show look ahead"            );
    flag.BoolVar(  &lookFlag  , "l"      , false    , "show look ahead (shorthand)");
    flag.BoolVar(  &blankFlag , "blank"  , false    , "blank walls"                );
    flag.BoolVar(  &blankFlag , "b"      , false    , "blank walls     (shorthand)");
    flag.StringVar(&outputName, "output" , ""       , "output ascii"               );
    flag.StringVar(&outputName, "o"      , ""       , "output ascii    (shorthand)");

    flag.Parse()

    if depthVal <  0 || depthVal > 100            {; depthVal = 100           ;}
    if fps      <  0 || fps      > 100000         {; fps      = 100000        ;}
    if height   <= 0 || height   > maxHeight      {; height   = maxHeight     ;}
    if width    <= 0 || width    > maxWidth       {; width    = maxWidth      ;}
    if minLen   <  0 || minLen   > height*width/3 {; minLen   = height*width/3;}

    setBool(&checkFlag, lookFlag);
    setInt( &depth    , depthVal);

    clrScreen()
    setCursorOff()
    go displayRoutine()

    for {
        switch {
            case fps ==    0: setInt(&delay,       0      )
            case fps <= 1000: setInt(&delay,    1000 / fps)
            default:          setInt(&delay, 1000000 / fps)
        }

        incInt(&numMazeCreated)
        if (getInt(&numMazeCreated) > 1 || seed == 0) {
            seed = time.Now().Nanosecond()
        }
        rand.Seed(int64(seed));

        var pathStartX int
        var pathStartY int

        createMaze(&pathStartX, &pathStartY); if showFlag {; updateMaze(0);  msSleep(1000); }
         solveMaze(&pathStartX, &pathStartY); if showFlag {; updateMaze(0);  msSleep(1000); }

        if getInt(&solveLength) >= minLen {
           break
        }
    }
    updateMaze(0)
    msSleep(100)
    restoreMaze()
    outputAsciiMaze()
    setCursorOn()
    putchar('\n')
    myStdout.Flush()
}


# Dual Shock 4 Visualize

Toy project to visualize music, on Linux systems, with a dual shock 4 controller.

## Requirements

This project is compiled with `CGO_ENABLED=1`. The included script (`run`) utilizes
`ffmpeg` to convert the audio to the internal format accepted by the program, that is,
`s16le` with a `44100` sample rate.

## Usage

    $ ./run <audio file>

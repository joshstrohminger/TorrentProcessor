'use strict';

const argv = require('yargs')
    .command('copy', 'Copy completed files', (yargs) => {
        return yargs
            .option('N', { description: 'Torrent name'})
            .option('L', { description: 'Category'})
            .option('F', { description: 'Content path (same as root path for multifile torrent)'})
            .option('R', { description: 'Root path (first torrent subdirectory path)'})
            .option('D', { description: 'Save path'})
            .option('C', { description: 'Number of files'})
            .option('Z', { description: 'Torrent size (bytes)'})
            .option('T', { description: 'Current tracker'})
            .option('I', { description: 'Info hash'})
            .demandOption(['N','L','F','R','D','C','Z','T','I']);
    }, copy)
    .demandCommand()
    .help('h')
    .alias('h', 'help')
    .argv;

function copy(argv) {
    console.log('Copying files...');
}

'use strict';

const argv = require('yargs')
    .command('copy', 'Copy completed files', (yargs) => {
        return yargs
            .option('n', { alias: 'N', demandOption: true, requiresArg: true, type: 'string', description: 'Torrent name'})
            .option('l', { alias: 'L', demandOption: true, requiresArg: true, type: 'string', description: 'Category', choices: ['Moviesingle','TvSingle']})
            .option('f', { alias: 'F', demandOption: true, requiresArg: true, type: 'string', description: 'Content path (same as root path for multifile torrent)'})
            .option('r', { alias: 'R', demandOption: true, requiresArg: true, type: 'string', description: 'Root path (first torrent subdirectory path)'})
            .option('d', { alias: 'D', demandOption: true, requiresArg: true, type: 'string', description: 'Save path'})
            .option('c', { alias: 'C', demandOption: true, requiresArg: true, type: 'string', description: 'Number of files'})
            .option('z', { alias: 'Z', demandOption: true, requiresArg: true, type: 'string', description: 'Torrent size (bytes)'})
            .option('t', { alias: 'T', demandOption: true, requiresArg: true, type: 'string', description: 'Current tracker'})
            .option('i', { alias: 'I', demandOption: true, requiresArg: true, type: 'string', description: 'Info hash'});
    }, copyCommandHandler)
    .demandCommand()
    .help('h')
    .alias('h', 'H')
    .alias('h', 'help')
    .argv;

function copyCommandHandler(argv) {
    console.log('Copying files...');
}

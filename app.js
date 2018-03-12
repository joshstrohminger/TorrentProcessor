'use strict';

const fs = require('fs');
const path = require('path');
const chalk = require('chalk');
const prettyBytes = require('pretty-bytes');
const argv = require('yargs')
    .command('copy', 'Copy completed files', yargs => {
        return yargs
            .option('n', { demandOption: true, requiresArg: true, type: 'string', alias: ['N','Name'], description: 'Torrent name'})
            .option('l', { demandOption: true, requiresArg: true, type: 'string', alias: ['L','Category'], description: 'Category', choices: ['Moviesingle','TvSingle']})
            .option('f', { demandOption: true, requiresArg: true, type: 'string', alias: ['F','ContentPath'], description: 'Content path (same as root path for multifile torrent)'})
            .option('r', { demandOption: true, requiresArg: true, type: 'string', alias: ['R','RootPath'], description: 'Root path (first torrent subdirectory path)'})
            .option('d', { demandOption: true, requiresArg: true, type: 'string', alias: ['D','SavePath'], description: 'Save path'})
            .option('c', { demandOption: true, requiresArg: true, type: 'number', alias: ['C','NumberOfFiles'], description: 'Number of files'})
            .option('z', { demandOption: true, requiresArg: true, type: 'number', alias: ['Z','Bytes'], description: 'Torrent size (bytes)'})
            .option('t', { demandOption: true, requiresArg: true, type: 'string', alias: ['T','Tracker'], description: 'Current tracker'})
            .option('i', { demandOption: true, requiresArg: true, type: 'string', alias: ['I','Hash'], description: 'Info hash'})
            .option('o', { demandOption: true, requiresArg: true, type: 'string', alias: ['O','OutputPath'], description: 'Output root directory'});
    }, copyCommandHandler)
    .demandCommand()
    .help('h')
    .alias('h', 'H')
    .alias('h', 'help')
    .argv;

function copyCommandHandler(argv) {
    try {
        switch(argv.Category) {
            case 'MovieSingle': return copyMoviesSingle(argv);
            case 'TvSingle': return copyTvSingle(argv);
            default: throw 'Unhandled category';
        }
    } catch(e) {
        processingErrorHandler(e);
    }
}

function processingErrorHandler(message) {
    console.error(chalk.red('Error: ' + message));
    errorPair('Retry', process.argv.map(arg => '"' + arg + '"').join(' '));
}

function copyMoviesSingle(argv) {
    if(argv.NumberOfFiles === 1) {
        console.log('Content: ' + argv.ContentPath);
    } else if(argv.NumberOfFiles > 1) {
        console.log('Multiple files at: ' + argv.ContentPath);
    }
}

function copyTvSingle(argv) {
    if(argv.NumberOfFiles === 1) {
        var info = parseTvName(argv.Name);
        var sourcePath = argv.ContentPath;
        if(!fs.existsSync(sourcePath)) throw "Source doesn't exist: '" + sourcePath + "'";
        var destinationName = info.Name + ' S' + info.Season + 'E' + info.Episode + path.extname(sourcePath);
        var destinationPath = path.normalize(path.join(argv.OutputPath, 'TV', destinationName));
        if(fs.existsSync(destinationPath)) throw "Destination already exists: '" + destinationPath + "'";
        logPair('Copying', prettyBytes(argv.Bytes) + 'bytes from "' + sourcePath + '" to "' + destinationPath + '"');
        fs.copyFileSync(sourcePath, destinationPath, fs.constants.COPYFILE_EXCL);
    } else if(argv.NumberOfFiles > 1) {
        console.log('Multiple files at: ' + argv.ContentPath);
    }
}

function parseTvName(name) {
    var parts = name.match(/^(.*?)S(\d+)\.?E(\d+)/i);
    if(!parts) throw 'Failed to parse TV name';
    logPair('Parsed', parts);
    return {
        Name: parts[1].replace(/(\.|\s)+/g, ' ').trim(),
        Season: pad(parseInt(parts[2]),2),
        Episode: pad(parseInt(parts[3]),2)
    };
}

function pad(n, width, padding) {
    padding = padding || '0';
    n = n + '';
    return n.length >= width ? n : new Array(width - n.length + 1).join(padding) + n;
}

function logPair(label, value) {
    console.log(label + ': ' + chalk.gray(value));
}

function errorPair(label, value) {
    console.error(label + ': ' + chalk.gray(value));
}

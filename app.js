'use strict';

const fs = require('fs');
const path = require('path');
const chalk = require('chalk');
const prettyBytes = require('pretty-bytes');
const winston = require('winston');

const env = process.env.NODE_ENV || 'development';
const logDir = path.join(__dirname, 'log');
const tsFormat = () => new Date().toISOString();
if (!fs.existsSync(logDir)) {
    fs.mkdirSync(logDir);
}
const log = new (winston.Logger)({
    transports: [
        new (winston.transports.Console)({
            timestamp: tsFormat,
            colorize: true,
            level: 'info'
        }),
        new (winston.transports.File)({
            filename: `${logDir}/log.json`,
            timestamp: tsFormat,
            tailable: true,
            maxsize: 1024 * 500,
            maxFiles: 15,
            level: env === 'development' ? 'debug' : 'info'
        })
    ]
});

const argv = require('yargs')
    .command('copy', 'Copy completed files', yargs => {
        return yargs
            .option('n', { demandOption: true, requiresArg: true, type: 'string', alias: ['N', 'Name'], description: 'Torrent name' })
            .option('l', { demandOption: true, requiresArg: true, type: 'string', alias: ['L', 'Category'], description: 'Category', choices: ['Moviesingle', 'TvSingle'] })
            .option('f', { demandOption: true, requiresArg: true, type: 'string', alias: ['F', 'ContentPath'], description: 'Content path (same as root path for multifile torrent)' })
            .option('r', { demandOption: true, requiresArg: true, type: 'string', alias: ['R', 'RootPath'], description: 'Root path (first torrent subdirectory path)' })
            .option('d', { demandOption: true, requiresArg: true, type: 'string', alias: ['D', 'SavePath'], description: 'Save path' })
            .option('c', { demandOption: true, requiresArg: true, type: 'number', alias: ['C', 'NumberOfFiles'], description: 'Number of files' })
            .option('z', { demandOption: true, requiresArg: true, type: 'number', alias: ['Z', 'Bytes'], description: 'Torrent size (bytes)' })
            .option('t', { demandOption: true, requiresArg: true, type: 'string', alias: ['T', 'Tracker'], description: 'Current tracker' })
            .option('i', { demandOption: true, requiresArg: true, type: 'string', alias: ['I', 'Hash'], description: 'Info hash' })
            .option('o', { demandOption: true, requiresArg: true, type: 'string', alias: ['O', 'OutputPath'], description: 'Output root directory' });
    }, copyCommandHandler)
    .demandCommand()
    .option('practice', {requiresArg: false, type: 'boolean', alias: ['p','P'], description: "Don't actually do anything, just print/log like it to see if it works"})
    .help('h')
    .alias('h', 'H')
    .alias('h', 'help')
    .argv;

function copyCommandHandler(argv) {
    try {
        switch (argv.Category) {
            case 'MovieSingle': return copyMoviesSingle(argv);
            case 'TvSingle': return copyTvSingle(argv);
            default: throw 'Unhandled category';
        }
    } catch (e) {
        processingErrorHandler(e);
    }
}

function processingErrorHandler(message) {
    log.error(message);
    log.error(process.argv.map(arg => '"' + arg + '"').join(' '));
}

function copyMoviesSingle(argv) {
    if(argv.NumberOfFiles <= 0) {
        throw 'NumberOfFiles must be greater than zero';
    }
    if (argv.NumberOfFiles === 1) {
        var name = parseMovieName(argv.Name);
        var sourcePath = argv.ContentPath;
        if (!fs.existsSync(sourcePath)) throw "Source doesn't exist: '" + sourcePath + "'";
        var destinationName = name + path.extname(sourcePath);
        var destinationPath = path.normalize(path.join(argv.OutputPath, 'Movies', destinationName));
        if (fs.existsSync(destinationPath)) throw "Destination already exists: '" + destinationPath + "'";
        logPair('Copying', prettyBytes(argv.Bytes) + 'bytes from "' + sourcePath + '" to "' + destinationPath + '"');
        if(!argv.practice) {
            fs.copyFileSync(sourcePath, destinationPath, fs.constants.COPYFILE_EXCL);
        }
    } else if (argv.NumberOfFiles === 2) {
        // assume the second file is english subtitles
        var name = parseMovieName(argv.Name);
        var files = getFiles(argv.ContentPath, {
            Video: file => !isSubtitleFile(file),
            Subtitles: file => isSubtitleFile(file)
        });
        var videoDestination = path.normalize(path.join(argv.OutputPath, 'Movies', name + path.extname(files.Video)));
        var subtitlesDestination = path.normalize(path.join(argv.OutputPath, 'Movies', name + '.en' + path.extname(files.Subtitles)));
        if (fs.existsSync(videoDestination)) throw "Video destination already exists: '" + videoDestination + "'";
        if (fs.existsSync(subtitlesDestination)) throw "Subtitles destination already exists: '" + subtitlesDestination + "'";
        logPair('Copying', prettyBytes(argv.Bytes) + 'bytes from "' + files.Video + '" to "' + videoDestination + '"');
        if(!argv.practice) {
            fs.copyFileSync(files.Video, videoDestination, fs.constants.COPYFILE_EXCL);
        }
        logPair('Copying', prettyBytes(argv.Bytes) + 'bytes from "' + files.Subtitles + '" to "' + subtitlesDestination + '"');
        if(!argv.practice) {
            fs.copyFileSync(files.Subtitles, subtitlesDestination, fs.constants.COPYFILE_EXCL);
        }
    } else {
        throw `Handling ${argv.NumberOfFiles} movie files is not yet supported`;
    }
}

function getFiles(directory, criteria) {
    let files = fs.readdirSync(directory);
    let foundFiles = {};
    for(var name in criteria) {
        let criterion = criteria[name];
        for(var file of files) {
            if(criterion(file)) {
                foundFiles[name] = file;
                break;
            }
        }
        if(!foundFiles[name]) {
            throw 'Failed to find file matching criteron: ' + name;
        }
    }
    return foundFiles;
}

const supportedSubtitleExtensions = ['.srt', '.smi', '.ssa', '.ass', '.vtt'];
function isSubtitleFile(file) {
    return supportedSubtitleExtensions.indexOf(path.extname().toLowerCase()) > -1;
}

function copyTvSingle(argv) {
    if (argv.NumberOfFiles === 1) {
        var info = parseTvName(argv.Name);
        var sourcePath = argv.ContentPath;
        if (!fs.existsSync(sourcePath)) throw "Source doesn't exist: '" + sourcePath + "'";
        var destinationName = info.Name + ' S' + info.Season + 'E' + info.Episode + path.extname(sourcePath);
        var destinationPath = path.normalize(path.join(argv.OutputPath, 'TV', destinationName));
        if (fs.existsSync(destinationPath)) throw "Destination already exists: '" + destinationPath + "'";
        logPair('Copying', prettyBytes(argv.Bytes) + 'bytes from "' + sourcePath + '" to "' + destinationPath + '"');
        if(!argv.practice) {
            fs.copyFileSync(sourcePath, destinationPath, fs.constants.COPYFILE_EXCL);
        }
    } else if (argv.NumberOfFiles > 1) {
        log.info('Multiple files at: ' + argv.ContentPath);
    }
}

function parseTvName(name) {
    var parts = name.match(/^(.*?)S(\d+)\.?E(\d+)/i);
    if (!parts) throw 'Failed to parse TV name';
    logPair('Parsed', parts);
    return {
        Name: parts[1].replace(/(\.|\s)+/g, ' ').trim(),
        Season: pad(parseInt(parts[2]), 2),
        Episode: pad(parseInt(parts[3]), 2)
    };
}

function parseMovieName(name) {
    var parts = name.match(/^(.*)\b(\d{4})\b/i);
    if(parts) {
        logPair('Parsed', parts);
        var info = {
            Name: parts[1].replace(/(\.|\s)+/g, ' ').trim(),
            Year: parts[2]
        };
        return `${info.Name} (${info.Year})`;
    }
    errorPair('Failed to parse movie name', name);
    var baseName = path.basename(name, path.extname(name));
    return baseName.replace(/(\.|\s)+/g, ' ');
}

function pad(n, width, padding) {
    padding = padding || '0';
    n = n + '';
    return n.length >= width ? n : new Array(width - n.length + 1).join(padding) + n;
}

function logPair(label, value) {
    log.info(label + ': ' + chalk.gray(value));
}

function errorPair(label, value) {
    log.error(label + ': ' + chalk.gray(value));
}

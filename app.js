'use strict';

const fs = require('fs');
const path = require('path');
const prettyBytes = require('pretty-bytes');
const winston = require('winston');
const properCase = require('proper-case');
const trueCasePath = require('true-case-path')

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

const supportedSubtitleExtensions = ['.srt', '.smi', '.ssa', '.ass', '.vtt'];

const parser = require('yargs')
    .command('copy', 'Copy completed files', yargs => {
        return yargs
            .option('n', { demandOption: true, requiresArg: true, type: 'string', alias: ['N', 'Name'], description: 'Torrent name' })
            .option('l', { demandOption: true, requiresArg: true, type: 'string', alias: ['L', 'Category'], description: 'Category', choices: ['MovieSingle', 'TvSingle', 'TvSeason'] })
            .option('f', { demandOption: true, requiresArg: true, type: 'string', alias: ['F', 'ContentPath'], description: 'Content path (same as root path for multifile torrent)' })
            .option('r', { demandOption: true, requiresArg: true, type: 'string', alias: ['R', 'RootPath'], description: 'Root path (first torrent subdirectory path)' })
            .option('d', { demandOption: true, requiresArg: true, type: 'string', alias: ['D', 'SavePath'], description: 'Save path' })
            .option('c', { demandOption: true, requiresArg: true, type: 'number', alias: ['C', 'NumberOfFiles'], description: 'Number of files' })
            .option('z', { demandOption: true, requiresArg: true, type: 'number', alias: ['Z', 'Bytes'], description: 'Torrent size (bytes)' })
            .option('t', { demandOption: true, requiresArg: true, type: 'string', alias: ['T', 'Tracker'], description: 'Current tracker' })
            .option('i', { demandOption: true, requiresArg: true, type: 'string', alias: ['I', 'Hash'], description: 'Info hash' })
            .option('o', { demandOption: true, requiresArg: true, type: 'string', alias: ['O', 'OutputPath'], description: 'Output root directory' });
    }, copyCommandHandler)
    .demandCommand(1, "Must provide a valid command")
    .strict()
    .option('practice', {requiresArg: false, type: 'boolean', alias: ['p','P'], description: "Don't actually do anything, just print/log like it to see if it works"})
    .help('h')
    .alias('h', 'H')
    .alias('h', 'help');
    
parser.parse(process.argv.slice(2), (err, argv, output) => {
    log.info(output);
    if(err) {
        processingErrorHandler(err);
    }
});

function copyCommandHandler(argv) {
    try {
        switch (argv.Category) {
            case 'MovieSingle': return copyMoviesSingle(argv);
            case 'TvSingle': return copyTvSingle(argv);
            case 'TvSeason': return copyTvSeason(argv);
            default: throw 'Unhandled category';
        }
    } catch (e) {
        processingErrorHandler(e);
    }
}

function processingErrorHandler(message) {
    log.info(process.argv.join(' '));
    log.error(message);
}

function copyMoviesSingle(argv) {
    if(argv.NumberOfFiles <= 0) {
        throw 'NumberOfFiles must be greater than zero';
    }
    if (argv.NumberOfFiles === 1) {
        var name = argv.Name; // don't parse movie names, just use the provided name
        var sourcePath = argv.ContentPath;
        if (!fs.existsSync(sourcePath)) throw `Source doesn't exist: "${sourcePath}"`;
        var destinationName = name + path.extname(sourcePath);
        var destinationPath = path.normalize(path.join(argv.OutputPath, 'Movies', destinationName));
        if (fs.existsSync(destinationPath)) throw `Destination already exists: "${destinationPath}"`;
        logPair('Copying', `${prettyBytes(argv.Bytes)} from "${sourcePath}" to "${destinationPath}"`);
        if(!argv.practice) {
            fs.copyFileSync(sourcePath, destinationPath, fs.constants.COPYFILE_EXCL);
        }
    } else {
        // assume the second file is english subtitles
        var name = argv.Name; // don't parse movie names, just use the provided name
        var files = getFiles(argv.ContentPath, {
            Video: file => !isSubtitleFile(file),
            Subtitles: file => isSubtitleFile(file)
        });
        if(!files.Video) {
            throw 'Failed to find video file';
        }
        var videoDestination = path.normalize(path.join(argv.OutputPath, 'Movies', name + path.extname(files.Video)));
        if (fs.existsSync(videoDestination)) throw `Video destination already exists: "${videoDestination}"`;
        logPair('Copying', `${prettyBytes(argv.Bytes)} from "${files.Video}" to "${videoDestination}"`);
        if(!argv.practice) {
            fs.copyFileSync(files.Video, videoDestination, fs.constants.COPYFILE_EXCL);
        }
        if(files.Subtitles) {
            var subtitlesDestination = path.normalize(path.join(argv.OutputPath, 'Movies', `${name}.en${path.extname(files.Subtitles)}`));
            if (fs.existsSync(subtitlesDestination)) throw `Subtitles destination already exists: "${subtitlesDestination}"`;
            logPair('Copying', `${prettyBytes(argv.Bytes)} from "${files.Subtitles}" to "${subtitlesDestination}"`);
            if(!argv.practice) {
                fs.copyFileSync(files.Subtitles, subtitlesDestination, fs.constants.COPYFILE_EXCL);
            }
        }
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

function isSubtitleFile(file) {
    return supportedSubtitleExtensions.indexOf(path.extname().toLowerCase()) > -1;
}

function createTvDestinationDirectory(info, sourcePath, outputPath) {
    if (!fs.existsSync(sourcePath)) throw "Source doesn't exist: '" + sourcePath + "'";
    var destinationDirectory = path.normalize(path.join(outputPath, 'TV', info.Name));

    // check if show folder exists and create it if necessary
    if(fs.existsSync(destinationDirectory)) {
        // ensure we get the correct case as it already exists in the file system
        info.Name = path.basename(trueCasePath(destinationDirectory));
    } else {
        log.info(`Creating directory '${destinationDirectory}'`);
        if(!argv.practice) {
            fs.mkdirSync(destinationDirectory);
        }
    }

    return destinationDirectory;
}

function copyTvSeason(argv) {
    if(argv.NumberOfFiles < 2) {
        log.error('Not multiple files at: ' + argv.ContentPath);
    } else {
        var info = parseSeasonName(argv.Name);
        var sourcePath = argv.ContentPath;
        var destinationDirectory = createTvDestinationDirectory(info, sourcePath, argv.OutputPath);
        
        fs.readdirSync(sourcePath).filter(file => path.extname(file) == '.mkv').forEach(file => {
            try {
                var fileInfo = parseTvName(file);
                var fileSourcePath = path.normalize(path.join(sourcePath, file));
                
                // Use season name in case the episode name is different
                var destinationFile = info.Name + ' S' + fileInfo.Season + 'E' + fileInfo.Episode + path.extname(fileSourcePath);
                var destinationPath = path.normalize(path.join(destinationDirectory, destinationFile));
                if (fs.existsSync(destinationPath)) throw "Destination already exists: '" + destinationPath + "'";
                logPair('Copying', `"${fileSourcePath}" to "${destinationPath}"`);
                if(!argv.practice) {
                    fs.copyFileSync(fileSourcePath, destinationPath, fs.constants.COPYFILE_EXCL);
                }
            } catch (e) {
                log.error(`Failed to parse '${file}'`, e);
            }
        });
    }
}

function copyTvSingle(argv) {
    if (argv.NumberOfFiles === 1) {
        var info = parseTvName(argv.Name);
        var sourcePath = argv.ContentPath;
        var destinationDirectory = createTvDestinationDirectory(info, sourcePath, argv.OutputPath);   
        var destinationFile = info.Name + ' S' + info.Season + 'E' + info.Episode + path.extname(sourcePath);
        var destinationPath = path.normalize(path.join(destinationDirectory, destinationFile));
        if (fs.existsSync(destinationPath)) throw "Destination already exists: '" + destinationPath + "'";
        logPair('Copying', `${prettyBytes(argv.Bytes)} from "${sourcePath}" to "${destinationPath}"`);
        if(!argv.practice) {
            fs.copyFileSync(sourcePath, destinationPath, fs.constants.COPYFILE_EXCL);
        }
    } else if (argv.NumberOfFiles > 1) {
        log.error('Multiple files at: ' + argv.ContentPath);
    }
}

function parseSeasonName(name) {
    var parts = name.match(/^(.*?)S(\d+)/i);
    if (!parts) throw `Failed to parse season name '${name}'`;
    logPair('Parsed', parts);
    return {
        Name: properCase(parts[1].replace(/(\.|\s)+/g, ' ').trim()),
        Season: pad(parseInt(parts[2]), 2)
    };
}

function parseTvName(name) {
    var parts = name.match(/^(.*?)S(\d+)\.?E(\d+)/i);
    if (!parts) throw `Failed to parse TV name '${name}'`;
    logPair('Parsed', parts);
    return {
        Name: properCase(parts[1].replace(/(\.|\s)+/g, ' ').trim()),
        Season: pad(parseInt(parts[2]), 2),
        Episode: pad(parseInt(parts[3]), 2)
    };
}

function parseMovieName(name) {
    var parts = name.match(/^(.*)\b(\d{4})\b/i);
    if(parts) {
        logPair('Parsed', parts);
        var info = {
            Name: properCase(parts[1].replace(/(\.|\s)+/g, ' ').trim()),
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
    log.info(label + ': ' + value);
}

function errorPair(label, value) {
    log.error(label + ': ' + value);
}

function toTitleCase(str)
{
    return str.replace(/\w\S*/g, function(txt){return txt.charAt(0).toUpperCase() + txt.substr(1).toLowerCase();});
}
